// Package core is the desktop app's controller. The Wails layer is a thin
// binding over this package so the logic stays testable without a GUI.
package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/AltairCA/ExitLagFree/internal/client/elevate"
	"github.com/AltairCA/ExitLagFree/internal/client/ipc"
	"github.com/AltairCA/ExitLagFree/internal/client/nodeapi"
	"github.com/AltairCA/ExitLagFree/internal/client/secrets"
	"github.com/AltairCA/ExitLagFree/internal/icmpping"
	"github.com/AltairCA/ExitLagFree/internal/netutil"
	"github.com/AltairCA/ExitLagFree/internal/probe"
	"github.com/AltairCA/ExitLagFree/internal/proto"
	"github.com/AltairCA/ExitLagFree/internal/version"
	"github.com/AltairCA/ExitLagFree/profiles"
)

type Node struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Host            string    `json:"host"`
	APIPort         int       `json:"api_port"`
	Fingerprint     string    `json:"fingerprint"`
	DeviceID        string    `json:"device_id"`
	DeviceName      string    `json:"device_name"`
	Address         string    `json:"address"`
	Gateway         string    `json:"gateway"`
	ServerPublicKey string    `json:"server_public_key"`
	Endpoint        string    `json:"endpoint"`
	ProbePort       int       `json:"probe_port"`
	MTU             int       `json:"mtu"`
	AddedAt         time.Time `json:"added_at"`
}

type settings struct {
	Nodes     []Node             `json:"nodes"`
	Selection profiles.Selection `json:"selection"`
	LastNode  string             `json:"last_node,omitempty"`
}

// HelperAPI is the subset of the helper IPC client used here (mockable).
type HelperAPI interface {
	Status(ctx context.Context) (proto.HelperStatus, error)
	Connect(ctx context.Context, req proto.ConnectRequest) (proto.HelperStatus, error)
	Disconnect(ctx context.Context) (proto.HelperStatus, error)
	Ping(ctx context.Context, req proto.PingRequest) (proto.PingResponse, error)
}

type Core struct {
	dir      string
	secrets  *secrets.Store
	helper   HelperAPI
	resolver *profiles.Resolver
	// bundledVersion reports the version of the helper shipped with the app
	// ("" if unknown); nil disables update detection.
	bundledVersion func() string

	mu sync.Mutex
	st settings
}

func New() (*Core, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	c, err := NewWithDir(filepath.Join(base, "ExitLagFree"), ipc.NewClient())
	if err != nil {
		return nil, err
	}
	var once sync.Once
	var v string
	c.bundledVersion = func() string {
		once.Do(func() {
			if p, err := elevate.FindHelper(); err == nil {
				v, _ = elevate.HelperVersion(p)
			}
		})
		return v
	}
	return c, nil
}

func (c *Core) bundledHelperVersion() string {
	if c.bundledVersion == nil {
		return ""
	}
	return c.bundledVersion()
}

func NewWithDir(dir string, helper HelperAPI) (*Core, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	c := &Core{
		dir:      dir,
		secrets:  secrets.New(dir),
		helper:   helper,
		resolver: &profiles.Resolver{CacheDir: filepath.Join(dir, "cache")},
	}
	b, err := os.ReadFile(c.settingsPath())
	if err == nil {
		if err := json.Unmarshal(b, &c.st); err != nil {
			return nil, fmt.Errorf("corrupt settings: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if len(c.st.Selection.Profiles) == 0 && len(c.st.Selection.Custom) == 0 {
		c.st.Selection.Profiles = []string{"cs2"}
	}
	return c, nil
}

func (c *Core) settingsPath() string { return filepath.Join(c.dir, "settings.json") }

func (c *Core) saveLocked() error {
	b, _ := json.MarshalIndent(c.st, "", "  ")
	tmp := c.settingsPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.settingsPath())
}

type State struct {
	Version         string `json:"version"`
	HelperInstalled bool   `json:"helper_installed"`
	HelperError     string `json:"helper_error,omitempty"`
	// HelperOutdated is set when the running helper differs from the one
	// bundled with this app (e.g. after the app was upgraded).
	HelperOutdated       bool               `json:"helper_outdated"`
	BundledHelperVersion string             `json:"bundled_helper_version,omitempty"`
	Status               proto.HelperStatus `json:"status"`
	Nodes                []Node             `json:"nodes"`
	Selection            profiles.Selection `json:"selection"`
	Profiles             []profiles.Profile `json:"profiles"`
	LastNode             string             `json:"last_node,omitempty"`
}

func (c *Core) State(ctx context.Context) State {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	s := State{Version: version.Version}
	st, err := c.helper.Status(ctx)
	if err != nil {
		s.HelperError = err.Error()
		s.Status.State = proto.StateDisconnected
	} else {
		s.HelperInstalled = true
		s.Status = st
		s.BundledHelperVersion = c.bundledHelperVersion()
		s.HelperOutdated = s.BundledHelperVersion != "" && st.Version != s.BundledHelperVersion
	}
	s.Profiles, _ = profiles.All()
	c.mu.Lock()
	s.Nodes = append([]Node{}, c.st.Nodes...)
	s.Selection = c.st.Selection
	s.LastNode = c.st.LastNode
	c.mu.Unlock()
	return s
}

func secretKey(nodeID, kind string) string { return "node/" + nodeID + "/" + kind }

// AddNode redeems a pairing link. The WireGuard private key is generated
// here and never leaves this machine.
func (c *Core) AddNode(ctx context.Context, rawLink string) (Node, error) {
	link, err := proto.ParsePairingLink(rawLink)
	if err != nil {
		return Node{}, err
	}
	priv, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return Node{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	resp, err := nodeapi.Pair(ctx, link, priv.PublicKey().String())
	if err != nil {
		return Node{}, err
	}
	n := Node{
		ID:              resp.DeviceID,
		Name:            firstNonEmpty(resp.NodeName, link.NodeName, link.Host),
		Host:            link.Host,
		APIPort:         link.Port,
		Fingerprint:     link.Fingerprint,
		DeviceID:        resp.DeviceID,
		DeviceName:      resp.DeviceName,
		Address:         resp.Address,
		Gateway:         resp.Gateway,
		ServerPublicKey: resp.ServerPublicKey,
		Endpoint:        resp.Endpoint,
		ProbePort:       resp.ProbePort,
		MTU:             resp.MTU,
		AddedAt:         time.Now(),
	}
	if err := c.secrets.Set(secretKey(n.ID, "private_key"), priv.String()); err != nil {
		return Node{}, fmt.Errorf("store private key: %w", err)
	}
	if err := c.secrets.Set(secretKey(n.ID, "token"), resp.DeviceToken); err != nil {
		return Node{}, fmt.Errorf("store device token: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.st.Nodes = append(c.st.Nodes, n)
	if c.st.LastNode == "" {
		c.st.LastNode = n.ID
	}
	return n, c.saveLocked()
}

func (c *Core) node(id string) (Node, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, n := range c.st.Nodes {
		if n.ID == id {
			return n, nil
		}
	}
	return Node{}, fmt.Errorf("unknown node %q", id)
}

func (c *Core) RenameNode(id, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.st.Nodes {
		if c.st.Nodes[i].ID == id {
			c.st.Nodes[i].Name = name
			return c.saveLocked()
		}
	}
	return fmt.Errorf("unknown node %q", id)
}

// RemoveNode disconnects if needed, tells the node to forget this device
// (best effort) and deletes local credentials.
func (c *Core) RemoveNode(ctx context.Context, id string) error {
	n, err := c.node(id)
	if err != nil {
		return err
	}
	if st, err := c.helper.Status(ctx); err == nil && st.NodeID == id && st.State != proto.StateDisconnected {
		_, _ = c.helper.Disconnect(ctx)
	}
	if token, err := c.secrets.Get(secretKey(id, "token")); err == nil {
		if api, err := nodeapi.New(n.Host, n.APIPort, n.Fingerprint, token); err == nil {
			uctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_ = api.Unpair(uctx)
			cancel()
		}
	}
	c.secrets.Delete(secretKey(id, "private_key"))
	c.secrets.Delete(secretKey(id, "token"))

	c.mu.Lock()
	defer c.mu.Unlock()
	kept := c.st.Nodes[:0]
	for _, x := range c.st.Nodes {
		if x.ID != id {
			kept = append(kept, x)
		}
	}
	c.st.Nodes = kept
	if c.st.LastNode == id {
		c.st.LastNode = ""
	}
	return c.saveLocked()
}

type NodePing struct {
	NodeID  string  `json:"node_id"`
	RTTMs   float64 `json:"rtt_ms"`
	LossPct float64 `json:"loss_pct"`
	Error   string  `json:"error,omitempty"`
}

// PingNodes measures latency to every node over the authenticated UDP probe.
func (c *Core) PingNodes(ctx context.Context) []NodePing {
	c.mu.Lock()
	nodes := append([]Node{}, c.st.Nodes...)
	c.mu.Unlock()
	out := make([]NodePing, len(nodes))
	var wg sync.WaitGroup
	for i, n := range nodes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = c.pingNode(ctx, n, 5)
		}()
	}
	wg.Wait()
	return out
}

func (c *Core) pingNode(ctx context.Context, n Node, count int) NodePing {
	r := NodePing{NodeID: n.ID}
	token, err := c.secrets.Get(secretKey(n.ID, "token"))
	if err != nil {
		r.Error = "missing credentials; remove and re-add this node"
		return r
	}
	addr := net.JoinHostPort(n.Host, strconv.Itoa(n.ProbePort))
	res, err := probe.Measure(ctx, addr, n.DeviceID, probe.KeyFromToken(token), count, 150*time.Millisecond, time.Second)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	r.LossPct = res.LossPct
	if res.Recv == 0 {
		r.Error = "no reply (node offline, port blocked, or device revoked)"
		return r
	}
	r.RTTMs = ms(res.AvgRTT)
	return r
}

func (c *Core) SetSelection(sel profiles.Selection) error {
	if _, err := profiles.ValidateCustom(sel.Custom); err != nil {
		return err
	}
	all, err := profiles.All()
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, p := range all {
		known[p.ID] = true
	}
	for _, id := range sel.Profiles {
		if !known[id] {
			return fmt.Errorf("unknown profile %q", id)
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.st.Selection = sel
	return c.saveLocked()
}

func (c *Core) Connect(ctx context.Context, nodeID string) (proto.HelperStatus, error) {
	n, err := c.node(nodeID)
	if err != nil {
		return proto.HelperStatus{}, err
	}
	c.mu.Lock()
	sel := c.st.Selection
	c.mu.Unlock()

	routes, err := c.resolver.Resolve(ctx, sel)
	if err != nil {
		return proto.HelperStatus{}, err
	}
	if len(routes) == 0 {
		return proto.HelperStatus{}, errors.New("select at least one game profile or custom route")
	}
	priv, err := c.secrets.Get(secretKey(n.ID, "private_key"))
	if err != nil {
		return proto.HelperStatus{}, errors.New("missing private key; remove and re-add this node")
	}
	st, err := c.helper.Connect(ctx, proto.ConnectRequest{
		NodeID:          n.ID,
		NodeName:        n.Name,
		PrivateKey:      priv,
		Address:         n.Address,
		Gateway:         n.Gateway,
		ServerPublicKey: n.ServerPublicKey,
		Endpoint:        n.Endpoint,
		MTU:             n.MTU,
		Routes:          netutil.Strings(routes),
	})
	if err != nil {
		return st, err
	}
	c.mu.Lock()
	c.st.LastNode = n.ID
	_ = c.saveLocked()
	c.mu.Unlock()
	return st, nil
}

func (c *Core) Disconnect(ctx context.Context) (proto.HelperStatus, error) {
	return c.helper.Disconnect(ctx)
}

type Comparison struct {
	Target string `json:"target"`
	// Direct is ICMP from this PC. When DirectViaTunnel is set the target is
	// currently routed through the node, so Direct is the real tunnelled RTT.
	DirectMs        float64 `json:"direct_ms"`
	DirectLoss      float64 `json:"direct_loss"`
	DirectError     string  `json:"direct_error,omitempty"`
	DirectViaTunnel bool    `json:"direct_via_tunnel"`
	NodeLegMs       float64 `json:"node_leg_ms"`
	NodeLegError    string  `json:"node_leg_error,omitempty"`
	NodeToTargetMs  float64 `json:"node_to_target_ms"`
	NodeTargetLoss  float64 `json:"node_target_loss"`
	NodeTargetError string  `json:"node_target_error,omitempty"`
	ViaNodeMs       float64 `json:"via_node_ms"`
}

// Compare estimates whether routing target through a node helps:
// direct RTT vs (PC->node probe RTT + node->target ICMP RTT).
func (c *Core) Compare(ctx context.Context, nodeID, target string) (Comparison, error) {
	n, err := c.node(nodeID)
	if err != nil {
		return Comparison{}, err
	}
	addr, err := netutil.ParsePrefixes([]string{target})
	if err != nil || len(addr) != 1 || addr[0].Bits() != 32 || !netutil.IsPublicIPv4(addr[0].Addr()) {
		return Comparison{}, errors.New("enter a public IPv4 address of a game server")
	}
	res := Comparison{Target: addr[0].Addr().String()}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		res.DirectMs, res.DirectLoss, res.DirectError = c.directPing(ctx, res.Target)
	}()
	go func() {
		defer wg.Done()
		p := c.pingNode(ctx, n, 5)
		res.NodeLegMs, res.NodeLegError = p.RTTMs, p.Error
	}()
	go func() {
		defer wg.Done()
		token, err := c.secrets.Get(secretKey(n.ID, "token"))
		if err != nil {
			res.NodeTargetError = "missing credentials"
			return
		}
		api, err := nodeapi.New(n.Host, n.APIPort, n.Fingerprint, token)
		if err != nil {
			res.NodeTargetError = err.Error()
			return
		}
		pr, err := api.Probe(ctx, res.Target)
		if err != nil {
			res.NodeTargetError = err.Error()
			return
		}
		res.NodeToTargetMs, res.NodeTargetLoss = pr.RTTMs, pr.LossPct
		if pr.Received == 0 {
			res.NodeTargetError = "target did not answer ICMP from the node"
		}
	}()
	wg.Wait()

	if res.NodeLegError == "" && res.NodeTargetError == "" {
		res.ViaNodeMs = res.NodeLegMs + res.NodeToTargetMs
	}
	if st, err := c.helper.Status(ctx); err == nil && st.State == proto.StateConnected {
		c.mu.Lock()
		sel := c.st.Selection
		c.mu.Unlock()
		if routes, err := c.resolver.Resolve(ctx, sel); err == nil {
			for _, r := range routes {
				if r.Contains(addr[0].Addr()) {
					res.DirectViaTunnel = true
					break
				}
			}
		}
	}
	return res, nil
}

func (c *Core) directPing(ctx context.Context, target string) (float64, float64, string) {
	resp, err := c.helper.Ping(ctx, proto.PingRequest{Targets: []string{target}, Count: 5})
	if err == nil && len(resp.Results) == 1 {
		r := resp.Results[0]
		if r.Error != "" {
			return 0, 0, r.Error
		}
		if r.LossPct >= 100 {
			return 0, r.LossPct, "target did not answer ICMP"
		}
		return r.RTTMs, r.LossPct, ""
	}
	// Without the helper, try an unprivileged ICMP socket (works on macOS).
	lr, lerr := icmpping.Ping(ctx, target, 5, 5*time.Second)
	if lerr != nil {
		if err != nil {
			return 0, 0, "install the helper to measure direct latency"
		}
		return 0, 0, lerr.Error()
	}
	if lr.Received == 0 {
		return 0, lr.LossPct, "target did not answer ICMP"
	}
	return ms(lr.RTT), lr.LossPct, ""
}

func (c *Core) InstallHelper(ctx context.Context) error {
	path, err := elevate.FindHelper()
	if err != nil {
		return err
	}
	if err := elevate.InstallHelper(path); err != nil {
		return err
	}
	return c.waitHelperVersion(ctx, c.bundledHelperVersion())
}

// waitHelperVersion waits until the helper answers with the given version
// (any version if empty). On an update the old helper may keep answering
// until the elevated installer stops it, so being reachable isn't enough.
func (c *Core) waitHelperVersion(ctx context.Context, want string) error {
	deadline := time.Now().Add(60 * time.Second)
	var last proto.HelperStatus
	var lastErr error
	for time.Now().Before(deadline) {
		sctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		last, lastErr = c.helper.Status(sctx)
		cancel()
		if lastErr == nil && (want == "" || last.Version == want) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if lastErr == nil {
		return fmt.Errorf("helper is still running version %q instead of %q; check that the prompt was approved", last.Version, want)
	}
	return errors.New("helper did not start; check that the installation prompt was approved")
}

func (c *Core) UninstallHelper(ctx context.Context) error {
	path, err := elevate.FindHelper()
	if err != nil {
		return err
	}
	if err := elevate.UninstallHelper(path); err != nil {
		return err
	}
	return c.waitHelper(ctx, false)
}

func (c *Core) waitHelper(ctx context.Context, wantUp bool) error {
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		sctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, err := c.helper.Status(sctx)
		cancel()
		if (err == nil) == wantUp {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if wantUp {
		return errors.New("helper did not start; check that the installation prompt was approved")
	}
	return errors.New("helper is still running")
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
