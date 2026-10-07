package probe

import (
	"context"
	"net"
	"testing"
	"time"
)

const devID = "0123456789abcdef"

func startServer(t *testing.T, key []byte) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := &Server{Lookup: func(id string) ([]byte, bool) {
		return key, id == devID
	}}
	go s.Serve(ctx, pc)
	return pc.LocalAddr().String()
}

func TestMeasure(t *testing.T) {
	key := KeyFromToken("elfd_secret")
	addr := startServer(t, key)
	res, err := Measure(context.Background(), addr, devID, key, 5, 10*time.Millisecond, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if res.Recv != 5 || res.LossPct != 0 || res.AvgRTT <= 0 {
		t.Fatalf("unexpected result %+v", res)
	}
}

func TestWrongKeyIsIgnored(t *testing.T) {
	addr := startServer(t, KeyFromToken("right"))
	res, err := Measure(context.Background(), addr, devID, KeyFromToken("wrong"), 3, 5*time.Millisecond, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if res.Recv != 0 || res.LossPct != 100 {
		t.Fatalf("server answered unauthenticated probe: %+v", res)
	}
}

func TestStaleTimestampRejected(t *testing.T) {
	key := KeyFromToken("k")
	s := &Server{Lookup: func(string) ([]byte, bool) { return key, true }, Now: time.Now}
	s.buckets = map[[8]byte]*bucket{}
	id, _ := ParseDeviceID(devID)
	old := encode(magicReq, id, 0, time.Now().Add(-time.Hour).UnixNano(), key)
	if _, ok := s.handle(old); ok {
		t.Fatal("stale probe answered")
	}
	fresh := encode(magicReq, id, 0, time.Now().UnixNano(), key)
	if _, ok := s.handle(fresh); !ok {
		t.Fatal("fresh probe not answered")
	}
}

func TestRateLimit(t *testing.T) {
	key := KeyFromToken("k")
	now := time.Unix(1_700_000_000, 0)
	s := &Server{Lookup: func(string) ([]byte, bool) { return key, true }, Now: func() time.Time { return now }}
	s.buckets = map[[8]byte]*bucket{}
	id, _ := ParseDeviceID(devID)
	answered := 0
	for i := 0; i < 50; i++ {
		if _, ok := s.handle(encode(magicReq, id, uint32(i), now.UnixNano(), key)); ok {
			answered++
		}
	}
	if answered != perDevice {
		t.Fatalf("answered %d, want %d", answered, perDevice)
	}
}
