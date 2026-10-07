package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/AltairCA/ExitLagFree/internal/icmpping"
	"github.com/AltairCA/ExitLagFree/internal/node/store"
	"github.com/AltairCA/ExitLagFree/internal/node/wg"
	"github.com/AltairCA/ExitLagFree/internal/pin"
	"github.com/AltairCA/ExitLagFree/internal/proto"
)

type harness struct {
	srv     *httptest.Server
	store   *store.Store
	api     *Server
	client  *http.Client
	changes int
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{store: st}
	h.api = New(Options{
		NodeName:        "test",
		PublicHost:      "203.0.113.9",
		WGPort:          51820,
		ProbePort:       51821,
		MTU:             1420,
		Subnet:          netip.MustParsePrefix("10.66.0.0/24"),
		Gateway:         netip.MustParseAddr("10.66.0.1"),
		MaxDevices:      5,
		ServerPublicKey: st.PrivateKey().PublicKey(),
	}, st, func() (map[wgtypes.Key]wg.PeerStats, error) { return nil, nil }, func() { h.changes++ })
	h.api.ping = func(context.Context, string) (icmpping.Result, error) {
		return icmpping.Result{RTT: 12 * time.Millisecond, Sent: 4, Received: 4}, nil
	}
	h.srv = httptest.NewTLSServer(h.api.Handler())
	t.Cleanup(h.srv.Close)
	fp := pin.Fingerprint(h.srv.Certificate().Raw)
	h.client, err = pin.HTTPClient(fp, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *harness) pair(t *testing.T, token string) (*http.Response, proto.PairResponse) {
	t.Helper()
	k, _ := wgtypes.GeneratePrivateKey()
	body, _ := json.Marshal(proto.PairRequest{Token: token, PublicKey: k.PublicKey().String()})
	resp, err := h.client.Post(h.srv.URL+"/v1/pair", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var pr proto.PairResponse
	_ = json.NewDecoder(resp.Body).Decode(&pr)
	return resp, pr
}

func (h *harness) get(t *testing.T, path, token string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, h.srv.URL+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestPairFlow(t *testing.T) {
	h := newHarness(t)
	tok, _, _ := h.store.CreateInvite("laptop", time.Minute)
	resp, pr := h.pair(t, tok)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if pr.Address != "10.66.0.2/32" || pr.Endpoint != "203.0.113.9:51820" || pr.DeviceToken == "" || h.changes != 1 {
		t.Fatalf("unexpected pair response %+v (changes=%d)", pr, h.changes)
	}

	if r := h.get(t, "/v1/me", pr.DeviceToken); r.StatusCode != http.StatusOK {
		t.Fatalf("me: %d", r.StatusCode)
	}
	if r := h.get(t, "/v1/me", "elfd_bogus"); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bogus token: %d", r.StatusCode)
	}
	if r := h.get(t, "/v1/probe?target=10.0.0.1", pr.DeviceToken); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("private probe target allowed: %d", r.StatusCode)
	}
	if r := h.get(t, "/v1/probe?target=8.8.8.8", pr.DeviceToken); r.StatusCode != http.StatusOK {
		t.Fatalf("probe: %d", r.StatusCode)
	}

	// The invite can't be reused.
	if resp, _ := h.pair(t, tok); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("reused invite: %d", resp.StatusCode)
	}
}

func TestBruteForceLockout(t *testing.T) {
	h := newHarness(t)
	for i := 0; i < 10; i++ {
		if resp, _ := h.pair(t, "guess"); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d", i, resp.StatusCode)
		}
	}
	tok, _, _ := h.store.CreateInvite("x", time.Minute)
	if resp, _ := h.pair(t, tok); resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected lockout, got %d", resp.StatusCode)
	}
}

func TestPinMismatchRejected(t *testing.T) {
	h := newHarness(t)
	c, _ := pin.HTTPClient(strings.Repeat("00", 32), time.Second)
	if _, err := c.Get(h.srv.URL + "/v1/me"); err == nil || !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Fatalf("expected pin failure, got %v", err)
	}
}
