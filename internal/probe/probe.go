// Package probe implements an authenticated UDP echo used to measure the
// client-to-node latency without needing ICMP privileges.
//
// Packet layout (40 bytes, both directions):
//
//	magic[4] | device_id[8] | seq[4] | unix_nanos[8] | hmac_sha256[16]
//
// Requests use magic "ELFQ", replies "ELFR". The node answers only packets
// whose HMAC verifies with the device's probe key and whose timestamp is
// recent, so the port is silent to scanners and useless for reflection.
package probe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net"
	"sync"
	"time"
)

const (
	PacketLen  = 40
	macOffset  = 24
	maxSkew    = 30 * time.Second
	perDevice  = 20 // replies per second per device
	magicLen   = 4
	idOffset   = 4
	seqOffset  = 12
	timeOffset = 16
)

var (
	magicReq  = [4]byte{'E', 'L', 'F', 'Q'}
	magicResp = [4]byte{'E', 'L', 'F', 'R'}

	errInvalid = errors.New("invalid probe packet")
)

// KeyFromToken derives the probe HMAC key from a device token. The node
// stores SHA-256(token) and uses the same bytes.
func KeyFromToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func encode(magic [4]byte, id [8]byte, seq uint32, ts int64, key []byte) []byte {
	b := make([]byte, PacketLen)
	copy(b[0:], magic[:])
	copy(b[idOffset:], id[:])
	binary.BigEndian.PutUint32(b[seqOffset:], seq)
	binary.BigEndian.PutUint64(b[timeOffset:], uint64(ts))
	copy(b[macOffset:], mac(b[:macOffset], key))
	return b
}

func mac(msg, key []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(msg)
	return h.Sum(nil)[:PacketLen-macOffset]
}

func verify(b, key []byte) bool {
	return hmac.Equal(b[macOffset:], mac(b[:macOffset], key))
}

func ParseDeviceID(s string) ([8]byte, error) {
	var id [8]byte
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 8 {
		return id, errors.New("device id must be 16 hex characters")
	}
	copy(id[:], b)
	return id, nil
}

// KeyLookup returns the probe key for a device ID (hex), if it exists.
type KeyLookup func(deviceID string) ([]byte, bool)

type Server struct {
	Lookup KeyLookup
	Now    func() time.Time

	mu      sync.Mutex
	buckets map[[8]byte]*bucket
}

type bucket struct {
	second int64
	count  int
}

// Serve answers probes on conn until ctx is cancelled.
func (s *Server) Serve(ctx context.Context, conn net.PacketConn) error {
	if s.Now == nil {
		s.Now = time.Now
	}
	s.buckets = map[[8]byte]*bucket{}
	go func() {
		<-ctx.Done()
		conn.Close()
	}()
	buf := make([]byte, 512)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return err
		}
		if reply, ok := s.handle(buf[:n]); ok {
			_, _ = conn.WriteTo(reply, addr)
		}
	}
}

func (s *Server) handle(b []byte) ([]byte, bool) {
	if len(b) != PacketLen || [4]byte(b[:magicLen]) != magicReq {
		return nil, false
	}
	var id [8]byte
	copy(id[:], b[idOffset:seqOffset])
	key, ok := s.Lookup(hex.EncodeToString(id[:]))
	if !ok || !verify(b, key) {
		return nil, false
	}
	ts := int64(binary.BigEndian.Uint64(b[timeOffset:]))
	now := s.Now()
	if d := now.Sub(time.Unix(0, ts)); d > maxSkew || d < -maxSkew {
		return nil, false
	}
	if !s.allow(id, now) {
		return nil, false
	}
	seq := binary.BigEndian.Uint32(b[seqOffset:])
	return encode(magicResp, id, seq, ts, key), true
}

func (s *Server) allow(id [8]byte, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec := now.Unix()
	bk := s.buckets[id]
	if bk == nil || bk.second != sec {
		if len(s.buckets) > 4096 {
			s.buckets = map[[8]byte]*bucket{}
		}
		bk = &bucket{second: sec}
		s.buckets[id] = bk
	}
	bk.count++
	return bk.count <= perDevice
}

type Result struct {
	AvgRTT  time.Duration
	MinRTT  time.Duration
	LossPct float64
	Sent    int
	Recv    int
}

// Measure sends count probes to addr spaced by interval and waits up to
// timeout after the last one for replies.
func Measure(ctx context.Context, addr, deviceID string, key []byte, count int, interval, timeout time.Duration) (Result, error) {
	id, err := ParseDeviceID(deviceID)
	if err != nil {
		return Result{}, err
	}
	if count <= 0 {
		count = 5
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp", addr)
	if err != nil {
		return Result{}, err
	}
	defer conn.Close()

	type sample struct{ rtt time.Duration }
	results := make(chan sample, count)
	sent := make([]time.Time, count)
	var mu sync.Mutex

	deadline := time.Now().Add(time.Duration(count)*interval + timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetReadDeadline(deadline)

	go func() {
		buf := make([]byte, 512)
		seen := map[uint32]bool{}
		for {
			n, err := conn.Read(buf)
			if err != nil {
				close(results)
				return
			}
			b := buf[:n]
			if n != PacketLen || [4]byte(b[:magicLen]) != magicResp || [8]byte(b[idOffset:seqOffset]) != id || !verify(b, key) {
				continue
			}
			seq := binary.BigEndian.Uint32(b[seqOffset:])
			if int(seq) >= count || seen[seq] {
				continue
			}
			seen[seq] = true
			mu.Lock()
			t0 := sent[seq]
			mu.Unlock()
			results <- sample{rtt: time.Since(t0)}
			if len(seen) == count {
				close(results)
				return
			}
		}
	}()

	for i := 0; i < count; i++ {
		now := time.Now()
		mu.Lock()
		sent[i] = now
		mu.Unlock()
		if _, err := conn.Write(encode(magicReq, id, uint32(i), now.UnixNano(), key)); err != nil {
			return Result{}, err
		}
		if i < count-1 {
			select {
			case <-ctx.Done():
				return Result{}, ctx.Err()
			case <-time.After(interval):
			}
		}
	}

	res := Result{Sent: count}
	var total time.Duration
	for s := range results {
		res.Recv++
		total += s.rtt
		if res.MinRTT == 0 || s.rtt < res.MinRTT {
			res.MinRTT = s.rtt
		}
	}
	if res.Recv > 0 {
		res.AvgRTT = total / time.Duration(res.Recv)
	}
	res.LossPct = 100 * float64(count-res.Recv) / float64(count)
	return res, nil
}
