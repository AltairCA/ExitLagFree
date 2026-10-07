package core

import (
	"context"
	"errors"
	"net"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/AltairCA/ExitLagFree/internal/client/secrets"
	"github.com/AltairCA/ExitLagFree/internal/node/api"
	"github.com/AltairCA/ExitLagFree/internal/node/store"
	"github.com/AltairCA/ExitLagFree/internal/node/wg"
	"github.com/AltairCA/ExitLagFree/internal/pin"
	"github.com/AltairCA/ExitLagFree/internal/probe"
	"github.com/AltairCA/ExitLagFree/internal/proto"
	"github.com/AltairCA/ExitLagFree/profiles"
)

type fakeHelper struct {
	connected *proto.ConnectRequest
	version   string
}

func (f *fakeHelper) Status(context.Context) (proto.HelperStatus, error) {
	if f.connected != nil {
		return proto.HelperStatus{Version: f.version, State: proto.StateConnected, NodeID: f.connected.NodeID}, nil
	}
	return proto.HelperStatus{Version: f.version, State: proto.StateDisconnected}, nil
}

func (f *fakeHelper) Connect(_ context.Context, req proto.ConnectRequest) (proto.HelperStatus, error) {
	f.connected = &req
	return proto.HelperStatus{State: proto.StateConnected, NodeID: req.NodeID}, nil
}

func (f *fakeHelper) Disconnect(context.Context) (proto.HelperStatus, error) {
	f.connected = nil
	return proto.HelperStatus{State: proto.StateDisconnected}, nil
}

func (f *fakeHelper) Ping(context.Context, proto.PingRequest) (proto.PingResponse, error) {
	return proto.PingResponse{}, errors.New("not installed")
}

// startNode runs the real node API and probe server in-process and returns
// a fresh invite link.
func startNode(t *testing.T) (*store.Store, func(name string) string) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ps := &probe.Server{Lookup: func(id string) ([]byte, bool) {
		d, ok := st.DeviceByID(id)
		return d.ProbeKey(), ok
	}}
	go ps.Serve(ctx, pc)

	a := api.New(api.Options{
		NodeName:        "test-node",
		PublicHost:      "127.0.0.1",
		WGPort:          51820,
		ProbePort:       pc.LocalAddr().(*net.UDPAddr).Port,
		MTU:             1420,
		Subnet:          netip.MustParsePrefix("10.66.0.0/24"),
		Gateway:         netip.MustParseAddr("10.66.0.1"),
		MaxDevices:      5,
		ServerPublicKey: st.PrivateKey().PublicKey(),
	}, st, func() (map[wgtypes.Key]wg.PeerStats, error) { return nil, nil }, func() {})
	srv := httptest.NewTLSServer(a.Handler())
	t.Cleanup(srv.Close)
	host, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)
	fp := pin.Fingerprint(srv.Certificate().Raw)

	return st, func(name string) string {
		tok, _, err := st.CreateInvite(name, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		return proto.PairingLink{Host: host, Port: port, Token: tok, Fingerprint: fp, NodeName: "test-node"}.String()
	}
}

func newCore(t *testing.T, h HelperAPI) *Core {
	t.Helper()
	dir := t.TempDir()
	c, err := NewWithDir(dir, h)
	if err != nil {
		t.Fatal(err)
	}
	c.secrets = secrets.NewFileOnly(dir)
	return c
}

func TestPairPingConnectRemove(t *testing.T) {
	st, invite := startNode(t)
	h := &fakeHelper{}
	c := newCore(t, h)
	ctx := context.Background()

	n, err := c.AddNode(ctx, invite("laptop"))
	if err != nil {
		t.Fatal(err)
	}
	if n.Name != "test-node" || n.DeviceName != "laptop" || n.Address != "10.66.0.2/32" {
		t.Fatalf("unexpected node %+v", n)
	}
	if len(st.Devices()) != 1 {
		t.Fatal("node did not register device")
	}

	pings := c.PingNodes(ctx)
	if len(pings) != 1 || pings[0].Error != "" || pings[0].RTTMs <= 0 {
		t.Fatalf("ping: %+v", pings)
	}

	if err := c.SetSelection(profiles.Selection{Profiles: []string{"cs2"}, Custom: []string{"1.2.3.4"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Connect(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	if h.connected == nil || len(h.connected.Routes) < 2 || h.connected.PrivateKey == "" {
		t.Fatalf("helper got %+v", h.connected)
	}
	priv, _ := wgtypes.ParseKey(h.connected.PrivateKey)
	if priv.PublicKey().String() != st.Devices()[0].PublicKey {
		t.Fatal("private key sent to helper doesn't match the registered public key")
	}

	if err := c.RemoveNode(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	if h.connected != nil {
		t.Fatal("remove did not disconnect")
	}
	if len(st.Devices()) != 0 {
		t.Fatal("remove did not unpair on the node")
	}
	if len(c.State(ctx).Nodes) != 0 {
		t.Fatal("node still listed locally")
	}
}

func TestRevokedDevicePingFails(t *testing.T) {
	st, invite := startNode(t)
	c := newCore(t, &fakeHelper{})
	n, err := c.AddNode(context.Background(), invite("pc"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Revoke(n.DeviceID); err != nil {
		t.Fatal(err)
	}
	p := c.PingNodes(context.Background())
	if p[0].Error == "" {
		t.Fatal("revoked device still gets probe replies")
	}
}

func TestHelperUpdateDetection(t *testing.T) {
	h := &fakeHelper{version: "1.0.0"}
	c := newCore(t, h)
	ctx := context.Background()

	if s := c.State(ctx); s.HelperOutdated {
		t.Fatal("outdated without a known bundled version")
	}
	c.bundledVersion = func() string { return "1.1.0" }
	if s := c.State(ctx); !s.HelperOutdated || s.BundledHelperVersion != "1.1.0" {
		t.Fatalf("expected outdated helper, got %+v", s)
	}

	short, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := c.waitHelperVersion(short, "1.1.0"); err == nil {
		t.Fatal("old helper accepted as updated")
	}

	h.version = "1.1.0"
	if err := c.waitHelperVersion(ctx, "1.1.0"); err != nil {
		t.Fatal(err)
	}
	if c.State(ctx).HelperOutdated {
		t.Fatal("still outdated after update")
	}
}

func TestSetSelectionValidates(t *testing.T) {
	c := newCore(t, &fakeHelper{})
	if err := c.SetSelection(profiles.Selection{Custom: []string{"192.168.0.1"}}); err == nil {
		t.Fatal("private custom route accepted")
	}
	if err := c.SetSelection(profiles.Selection{Profiles: []string{"nope"}}); err == nil {
		t.Fatal("unknown profile accepted")
	}
}
