// Package helper implements the privileged exitlag-helper service: it owns
// the tunnel, programs routes and performs ICMP measurements on behalf of the
// unprivileged desktop app.
package helper

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/AltairCA/ExitLagFree/internal/client/tunnel"
	"github.com/AltairCA/ExitLagFree/internal/icmpping"
	"github.com/AltairCA/ExitLagFree/internal/netutil"
	"github.com/AltairCA/ExitLagFree/internal/proto"
	"github.com/AltairCA/ExitLagFree/internal/version"
)

const (
	maxRoutes      = 20000
	maxPingTargets = 32
)

type Helper struct {
	mu          sync.Mutex
	tun         *tunnel.Tunnel
	state       proto.TunnelState
	lastErr     string
	nodeID      string
	nodeName    string
	connectedAt time.Time
}

func New() *Helper {
	return &Helper{state: proto.StateDisconnected}
}

func (h *Helper) Status() proto.HelperStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := proto.HelperStatus{
		Version:     version.Version,
		State:       h.state,
		Error:       h.lastErr,
		NodeID:      h.nodeID,
		NodeName:    h.nodeName,
		ConnectedAt: h.connectedAt,
	}
	if h.tun != nil {
		st := h.tun.Stats()
		s.Interface = h.tun.Name()
		s.RxBytes, s.TxBytes, s.LastHandshake = st.RxBytes, st.TxBytes, st.LastHandshake
		s.Routes = h.tun.RouteCount()
	}
	return s
}

func (h *Helper) Connect(ctx context.Context, req proto.ConnectRequest) error {
	cfg, err := buildConfig(ctx, req)
	if err != nil {
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.tun != nil {
		h.tun.Close()
		h.tun = nil
	}
	h.state, h.lastErr = proto.StateConnecting, ""
	h.nodeID, h.nodeName = req.NodeID, req.NodeName

	t, err := tunnel.Start(cfg)
	if err != nil {
		h.state, h.lastErr = proto.StateError, err.Error()
		log.Printf("helper: connect to %s failed: %v", req.NodeName, err)
		return err
	}
	h.tun = t
	h.state = proto.StateConnected
	h.connectedAt = time.Now()
	log.Printf("helper: tunnel %s up to %s (%s) with %d routes", t.Name(), req.NodeName, cfg.Endpoint, len(cfg.Routes))
	return nil
}

func (h *Helper) Disconnect() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.tun != nil {
		h.tun.Close()
		h.tun = nil
		log.Printf("helper: tunnel down")
	}
	h.state, h.lastErr = proto.StateDisconnected, ""
	h.nodeID, h.nodeName = "", ""
	h.connectedAt = time.Time{}
	return nil
}

func (h *Helper) Ping(ctx context.Context, req proto.PingRequest) proto.PingResponse {
	targets := req.Targets
	if len(targets) > maxPingTargets {
		targets = targets[:maxPingTargets]
	}
	count := req.Count
	if count <= 0 || count > 20 {
		count = 4
	}
	results := make([]proto.PingResult, len(targets))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, target := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r := proto.PingResult{Target: target}
			res, err := icmpping.Ping(ctx, target, count, req.Timeout)
			if err != nil {
				r.Error = err.Error()
			} else {
				r.RTTMs = float64(res.RTT.Microseconds()) / 1000
				r.LossPct = res.LossPct
			}
			results[i] = r
		}()
	}
	wg.Wait()
	return proto.PingResponse{Results: results}
}

func buildConfig(ctx context.Context, req proto.ConnectRequest) (tunnel.Config, error) {
	var cfg tunnel.Config
	var err error
	if cfg.PrivateKey, err = wgtypes.ParseKey(req.PrivateKey); err != nil {
		return cfg, errors.New("invalid private key")
	}
	if cfg.ServerPublicKey, err = wgtypes.ParseKey(req.ServerPublicKey); err != nil {
		return cfg, errors.New("invalid server public key")
	}
	if cfg.Address, err = netip.ParsePrefix(req.Address); err != nil {
		return cfg, fmt.Errorf("invalid address %q", req.Address)
	}
	if cfg.Gateway, err = netip.ParseAddr(req.Gateway); err != nil {
		return cfg, fmt.Errorf("invalid gateway %q", req.Gateway)
	}
	if cfg.Endpoint, err = resolveEndpoint(ctx, req.Endpoint); err != nil {
		return cfg, err
	}
	cfg.MTU = req.MTU
	if cfg.MTU < 1280 || cfg.MTU > 1500 {
		cfg.MTU = proto.DefaultMTU
	}

	if len(req.Routes) > maxRoutes {
		return cfg, fmt.Errorf("too many routes (%d > %d)", len(req.Routes), maxRoutes)
	}
	routes, err := netutil.ParsePrefixes(req.Routes)
	if err != nil {
		return cfg, err
	}
	for _, r := range routes {
		if err := checkRoute(r); err != nil {
			return cfg, err
		}
	}
	cfg.Routes = netutil.Exclude(netutil.Merge(routes), cfg.Endpoint.Addr())
	return cfg, nil
}

// checkRoute refuses prefixes that would capture LAN, loopback or other
// non-public space; those can't be relayed by a node anyway and would
// break local networking.
func checkRoute(p netip.Prefix) error {
	if p.Bits() < 4 {
		return fmt.Errorf("route %s is too broad", p)
	}
	for _, np := range netutil.NonPublicV4 {
		if p.Overlaps(np) {
			return fmt.Errorf("route %s overlaps non-public range %s", p, np)
		}
	}
	return nil
}

func resolveEndpoint(ctx context.Context, endpoint string) (netip.AddrPort, error) {
	host, portStr, err := net.SplitHostPort(endpoint)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("invalid endpoint %q", endpoint)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return netip.AddrPort{}, fmt.Errorf("invalid endpoint port %q", portStr)
	}
	if a, err := netip.ParseAddr(host); err == nil {
		return netip.AddrPortFrom(a.Unmap(), uint16(port)), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip4", host)
	if err != nil || len(addrs) == 0 {
		return netip.AddrPort{}, fmt.Errorf("resolve %s: %v", host, err)
	}
	return netip.AddrPortFrom(addrs[0].Unmap(), uint16(port)), nil
}
