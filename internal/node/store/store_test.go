package store

import (
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func newKey(t *testing.T) string {
	k, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	return k.PublicKey().String()
}

var subnet = netip.MustParsePrefix("10.66.0.0/24")

func TestInviteIsSingleUse(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := s.CreateInvite("laptop", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	d, devTok, err := s.Redeem(tok, newKey(t), subnet, 10)
	if err != nil {
		t.Fatal(err)
	}
	if d.Address != "10.66.0.2" || d.Name != "laptop" {
		t.Fatalf("unexpected device %+v", d)
	}
	if got, ok := s.DeviceByToken(devTok); !ok || got.ID != d.ID {
		t.Fatal("device token lookup failed")
	}
	if _, _, err := s.Redeem(tok, newKey(t), subnet, 10); !errors.Is(err, ErrInvalidInvite) {
		t.Fatalf("reuse: got %v", err)
	}
}

func TestInviteExpires(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "state.json"))
	now := time.Now()
	s.now = func() time.Time { return now }
	tok, _, _ := s.CreateInvite("x", time.Minute)
	now = now.Add(2 * time.Minute)
	if _, _, err := s.Redeem(tok, newKey(t), subnet, 10); !errors.Is(err, ErrInvalidInvite) {
		t.Fatalf("got %v", err)
	}
}

func TestDeviceLimitAndRevoke(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "state.json"))
	tok, _, _ := s.CreateInvite("a", time.Minute)
	d, _, err := s.Redeem(tok, newKey(t), subnet, 1)
	if err != nil {
		t.Fatal(err)
	}
	tok2, _, _ := s.CreateInvite("b", time.Minute)
	if _, _, err := s.Redeem(tok2, newKey(t), subnet, 1); !errors.Is(err, ErrDeviceLimit) {
		t.Fatalf("got %v", err)
	}
	if err := s.Revoke(d.ID); err != nil {
		t.Fatal(err)
	}
	tok3, _, _ := s.CreateInvite("c", time.Minute)
	d3, _, err := s.Redeem(tok3, newKey(t), subnet, 1)
	if err != nil {
		t.Fatal(err)
	}
	if d3.Address != "10.66.0.2" {
		t.Fatalf("address not reused: %s", d3.Address)
	}
}

func TestDuplicateKeyRejected(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "state.json"))
	key := newKey(t)
	tok, _, _ := s.CreateInvite("a", time.Minute)
	if _, _, err := s.Redeem(tok, key, subnet, 10); err != nil {
		t.Fatal(err)
	}
	tok2, _, _ := s.CreateInvite("b", time.Minute)
	if _, _, err := s.Redeem(tok2, key, subnet, 10); !errors.Is(err, ErrKeyInUse) {
		t.Fatalf("got %v", err)
	}
}

func TestPersistenceAndSecretsHashed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, _ := Open(path)
	tok, _, _ := s.CreateInvite("a", time.Minute)
	_, devTok, err := s.Redeem(tok, newKey(t), subnet, 10)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	for _, secret := range []string{tok, devTok} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("plaintext secret written to disk")
		}
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("state file mode %v", fi.Mode().Perm())
	}
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if s2.PrivateKey() != s.PrivateKey() {
		t.Fatal("private key not persisted")
	}
	if _, ok := s2.DeviceByToken(devTok); !ok {
		t.Fatal("device not persisted")
	}
}
