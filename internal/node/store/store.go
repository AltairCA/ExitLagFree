// Package store persists the node's WireGuard key, paired devices and
// pending invites. Secrets (invite and device tokens) are stored only as
// SHA-256 hashes.
package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

var (
	// ErrInvalidInvite deliberately doesn't distinguish unknown, used and
	// expired tokens so callers can't probe invite state.
	ErrInvalidInvite = errors.New("invalid or expired invite")
	ErrDeviceLimit   = errors.New("device limit reached on this node")
	ErrKeyInUse      = errors.New("public key already registered")
	ErrSubnetFull    = errors.New("tunnel subnet has no free addresses")
	ErrNotFound      = errors.New("device not found")
	ErrBadKey        = errors.New("invalid WireGuard public key")
)

type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	PublicKey string    `json:"public_key"`
	Address   string    `json:"address"`
	TokenHash string    `json:"token_hash"`
	CreatedAt time.Time `json:"created_at"`
}

// ProbeKey is the HMAC key for the UDP latency probe. Clients derive the
// same value by hashing their device token.
func (d Device) ProbeKey() []byte {
	b, _ := hex.DecodeString(d.TokenHash)
	return b
}

type Invite struct {
	TokenHash string    `json:"token_hash"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type state struct {
	PrivateKey string   `json:"private_key"`
	Devices    []Device `json:"devices"`
	Invites    []Invite `json:"invites"`
}

type Store struct {
	path string
	now  func() time.Time

	mu sync.RWMutex
	st state
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, now: time.Now}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		key, err := wgtypes.GeneratePrivateKey()
		if err != nil {
			return nil, err
		}
		s.st.PrivateKey = key.String()
		if err := s.save(); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(b, &s.st); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) PrivateKey() wgtypes.Key {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k, _ := wgtypes.ParseKey(s.st.PrivateKey)
	return k
}

// CreateInvite returns a fresh single-use token. Only its hash is stored.
func (s *Store) CreateInvite(name string, ttl time.Duration) (string, Invite, error) {
	token := randomToken(32)
	inv := Invite{
		TokenHash: Hash(token),
		Name:      name,
		CreatedAt: s.now(),
		ExpiresAt: s.now().Add(ttl),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	s.st.Invites = append(s.st.Invites, inv)
	return token, inv, s.save()
}

// Redeem consumes an invite and registers a device. The invite is deleted
// even if registration fails afterwards, so a leaked link can't be retried.
func (s *Store) Redeem(token, publicKey string, subnet netip.Prefix, maxDevices int) (Device, string, error) {
	pub, err := wgtypes.ParseKey(publicKey)
	if err != nil {
		return Device{}, "", ErrBadKey
	}
	h := Hash(token)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	idx := -1
	for i, inv := range s.st.Invites {
		if inv.TokenHash == h {
			idx = i
			break
		}
	}
	if idx < 0 {
		return Device{}, "", ErrInvalidInvite
	}
	inv := s.st.Invites[idx]
	s.st.Invites = append(s.st.Invites[:idx], s.st.Invites[idx+1:]...)
	if err := s.save(); err != nil {
		return Device{}, "", err
	}

	if len(s.st.Devices) >= maxDevices {
		return Device{}, "", ErrDeviceLimit
	}
	for _, d := range s.st.Devices {
		if d.PublicKey == pub.String() {
			return Device{}, "", ErrKeyInUse
		}
	}
	addr, err := s.allocateLocked(subnet)
	if err != nil {
		return Device{}, "", err
	}
	deviceToken := "elfd_" + randomToken(32)
	d := Device{
		ID:        hex.EncodeToString(randomBytes(8)),
		Name:      inv.Name,
		PublicKey: pub.String(),
		Address:   addr.String(),
		TokenHash: Hash(deviceToken),
		CreatedAt: s.now(),
	}
	s.st.Devices = append(s.st.Devices, d)
	return d, deviceToken, s.save()
}

func (s *Store) allocateLocked(subnet netip.Prefix) (netip.Addr, error) {
	used := map[netip.Addr]bool{}
	for _, d := range s.st.Devices {
		if a, err := netip.ParseAddr(d.Address); err == nil {
			used[a] = true
		}
	}
	gateway := subnet.Addr().Next()
	for a := gateway.Next(); subnet.Contains(a); a = a.Next() {
		if !subnet.Contains(a.Next()) {
			break // broadcast address
		}
		if !used[a] {
			return a, nil
		}
	}
	return netip.Addr{}, ErrSubnetFull
}

func (s *Store) DeviceByToken(token string) (Device, bool) {
	h := Hash(token)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.st.Devices {
		if d.TokenHash == h {
			return d, true
		}
	}
	return Device{}, false
}

func (s *Store) DeviceByID(id string) (Device, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.st.Devices {
		if d.ID == id {
			return d, true
		}
	}
	return Device{}, false
}

func (s *Store) Devices() []Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Device(nil), s.st.Devices...)
}

func (s *Store) Revoke(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, d := range s.st.Devices {
		if d.ID == id {
			s.st.Devices = append(s.st.Devices[:i], s.st.Devices[i+1:]...)
			return s.save()
		}
	}
	return ErrNotFound
}

func (s *Store) PendingInvites() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	return len(s.st.Invites)
}

func (s *Store) pruneLocked() {
	now := s.now()
	kept := s.st.Invites[:0]
	for _, inv := range s.st.Invites {
		if now.Before(inv.ExpiresAt) {
			kept = append(kept, inv)
		}
	}
	s.st.Invites = kept
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func Hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func randomToken(n int) string {
	return base64.RawURLEncoding.EncodeToString(randomBytes(n))
}
