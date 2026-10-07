package node

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"path/filepath"
	"strconv"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/AltairCA/ExitLagFree/internal/node/admin"
	"github.com/AltairCA/ExitLagFree/internal/node/api"
	"github.com/AltairCA/ExitLagFree/internal/node/firewall"
	"github.com/AltairCA/ExitLagFree/internal/node/store"
	"github.com/AltairCA/ExitLagFree/internal/node/wg"
	"github.com/AltairCA/ExitLagFree/internal/pin"
	"github.com/AltairCA/ExitLagFree/internal/probe"
	"github.com/AltairCA/ExitLagFree/internal/proto"
	"github.com/AltairCA/ExitLagFree/internal/version"
)

const (
	DefaultInviteTTL = 15 * time.Minute
	MaxInviteTTL     = 7 * 24 * time.Hour
)

type daemon struct {
	cfg         Config
	store       *store.Store
	wg          wg.Manager
	fwBackend   string
	fingerprint string
}

// Run starts the node and blocks until ctx is cancelled.
func Run(ctx context.Context, cfg Config) error {
	subnet, err := cfg.SubnetPrefix()
	if err != nil {
		return err
	}
	st, err := store.Open(filepath.Join(cfg.DataDir, "state.json"))
	if err != nil {
		return fmt.Errorf("open state: %w", err)
	}
	cert, fp, err := pin.LoadOrCreate(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("tls cert: %w", err)
	}
	d := &daemon{cfg: cfg, store: st, fingerprint: fp}

	d.wg, err = wg.Setup(wg.Options{
		Interface:  cfg.Interface,
		PrivateKey: st.PrivateKey(),
		ListenPort: cfg.WGPort,
		Address:    netip.PrefixFrom(cfg.Gateway(), subnet.Bits()),
		MTU:        cfg.MTU,
	})
	if err != nil {
		return fmt.Errorf("wireguard: %w", err)
	}
	defer d.wg.Close()

	rules := firewall.Rules{
		Interface:       cfg.Interface,
		Subnet:          subnet,
		Gateway:         cfg.Gateway(),
		EgressAllowlist: cfg.Allowlist(),
	}
	d.fwBackend, err = firewall.Apply(rules)
	if err != nil {
		return fmt.Errorf("firewall: %w", err)
	}
	defer firewall.Remove(rules)

	if err := d.syncPeers(); err != nil {
		return fmt.Errorf("sync peers: %w", err)
	}

	errc := make(chan error, 3)

	probeConn, err := net.ListenPacket("udp", ":"+strconv.Itoa(cfg.ProbePort))
	if err != nil {
		return fmt.Errorf("probe listener: %w", err)
	}
	ps := &probe.Server{Lookup: func(id string) ([]byte, bool) {
		dev, ok := st.DeviceByID(id)
		if !ok {
			return nil, false
		}
		return dev.ProbeKey(), true
	}}
	go func() { errc <- ps.Serve(ctx, probeConn) }()

	apiSrv := api.New(api.Options{
		NodeName:        cfg.NodeName,
		PublicHost:      cfg.PublicHost,
		WGPort:          cfg.WGPort,
		ProbePort:       cfg.ProbePort,
		MTU:             cfg.MTU,
		Subnet:          subnet,
		Gateway:         cfg.Gateway(),
		MaxDevices:      cfg.MaxDevices,
		ServerPublicKey: st.PrivateKey().PublicKey(),
	}, st, d.wg.Stats, d.onChange)
	httpsSrv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.APIPort),
		Handler:           apiSrv.Handler(),
		TLSConfig:         &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13},
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    8 << 10,
		ErrorLog:          log.New(discardTLSNoise{}, "", 0),
	}
	go func() {
		if err := httpsSrv.ListenAndServeTLS("", ""); !errors.Is(err, http.ErrServerClosed) {
			errc <- fmt.Errorf("api: %w", err)
		}
	}()

	adminLn, err := admin.Listen(cfg.AdminSocket)
	if err != nil {
		return fmt.Errorf("admin socket: %w", err)
	}
	adminSrv := &http.Server{Handler: admin.Handler(d), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := adminSrv.Serve(adminLn); !errors.Is(err, http.ErrServerClosed) {
			errc <- fmt.Errorf("admin: %w", err)
		}
	}()

	log.Printf("exitlag-node %s up: wg=%s/%s udp:%d api=tcp:%d probe=udp:%d firewall=%s devices=%d",
		version.Version, cfg.Interface, d.wg.Backend(), cfg.WGPort, cfg.APIPort, cfg.ProbePort, d.fwBackend, len(st.Devices()))
	log.Printf("certificate fingerprint: %s", fp)

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errc:
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpsSrv.Shutdown(shutdownCtx)
	_ = adminSrv.Shutdown(shutdownCtx)
	return runErr
}

func (d *daemon) syncPeers() error {
	var peers []wg.Peer
	for _, dev := range d.store.Devices() {
		key, err := wgtypes.ParseKey(dev.PublicKey)
		if err != nil {
			continue
		}
		addr, err := netip.ParseAddr(dev.Address)
		if err != nil {
			continue
		}
		peers = append(peers, wg.Peer{PublicKey: key, Address: addr})
	}
	return d.wg.Sync(peers)
}

func (d *daemon) onChange() {
	if err := d.syncPeers(); err != nil {
		log.Printf("sync peers: %v", err)
	}
}

func (d *daemon) Invite(name string, ttl time.Duration) (proto.InviteResponse, error) {
	if name == "" {
		name = "device"
	}
	if len(name) > 64 {
		return proto.InviteResponse{}, errors.New("name too long (max 64 characters)")
	}
	if ttl <= 0 {
		ttl = DefaultInviteTTL
	}
	if ttl > MaxInviteTTL {
		return proto.InviteResponse{}, fmt.Errorf("ttl may not exceed %s", MaxInviteTTL)
	}
	if n := len(d.store.Devices()); n >= d.cfg.MaxDevices {
		return proto.InviteResponse{}, fmt.Errorf("node already has %d/%d devices; revoke one or raise max_devices", n, d.cfg.MaxDevices)
	}
	token, inv, err := d.store.CreateInvite(name, ttl)
	if err != nil {
		return proto.InviteResponse{}, err
	}
	link := proto.PairingLink{
		Host:        d.cfg.PublicHost,
		Port:        d.cfg.APIPort,
		Token:       token,
		Fingerprint: d.fingerprint,
		NodeName:    d.cfg.NodeName,
	}
	log.Printf("admin: created invite %q valid until %s", name, inv.ExpiresAt.Format(time.RFC3339))
	return proto.InviteResponse{Link: link.String(), ExpiresAt: inv.ExpiresAt}, nil
}

func (d *daemon) Devices() ([]proto.DeviceInfo, error) {
	stats, err := d.wg.Stats()
	if err != nil {
		return nil, err
	}
	var out []proto.DeviceInfo
	for _, dev := range d.store.Devices() {
		info := proto.DeviceInfo{
			ID:        dev.ID,
			Name:      dev.Name,
			Address:   dev.Address,
			PublicKey: dev.PublicKey,
			CreatedAt: dev.CreatedAt,
		}
		if key, err := wgtypes.ParseKey(dev.PublicKey); err == nil {
			s := stats[key]
			info.LastHandshake, info.RxBytes, info.TxBytes, info.Endpoint = s.LastHandshake, s.RxBytes, s.TxBytes, s.Endpoint
		}
		out = append(out, info)
	}
	return out, nil
}

func (d *daemon) Revoke(id string) error {
	if err := d.store.Revoke(id); err != nil {
		return err
	}
	log.Printf("admin: revoked device %s", id)
	return d.syncPeers()
}

func (d *daemon) Status() proto.NodeStatus {
	return proto.NodeStatus{
		NodeName:       d.cfg.NodeName,
		Version:        version.Version,
		PublicHost:     d.cfg.PublicHost,
		Interface:      d.cfg.Interface,
		WGBackend:      d.wg.Backend(),
		Firewall:       d.fwBackend,
		Devices:        len(d.store.Devices()),
		MaxDevices:     d.cfg.MaxDevices,
		PendingInvites: d.store.PendingInvites(),
		Fingerprint:    d.fingerprint,
	}
}

// discardTLSNoise drops the "TLS handshake error" lines internet scanners
// generate constantly against any open TLS port.
type discardTLSNoise struct{}

func (discardTLSNoise) Write(p []byte) (int, error) { return len(p), nil }
