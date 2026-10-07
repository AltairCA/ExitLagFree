// Package tunnel runs an in-process wireguard-go device on a TUN interface
// and points only the selected game prefixes at it (split tunnelling).
package tunnel

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type Config struct {
	PrivateKey      wgtypes.Key
	ServerPublicKey wgtypes.Key
	Address         netip.Prefix   // this device's tunnel address, e.g. 10.66.0.5/32
	Gateway         netip.Addr     // node's tunnel address, e.g. 10.66.0.1
	Endpoint        netip.AddrPort // node's public WireGuard endpoint
	MTU             int
	Routes          []netip.Prefix // must not contain Endpoint.Addr()
}

func (c Config) validate() error {
	if !c.Address.IsValid() || !c.Address.Addr().Is4() {
		return errors.New("tunnel address must be IPv4")
	}
	if !c.Gateway.Is4() || !c.Endpoint.IsValid() {
		return errors.New("gateway and endpoint are required")
	}
	if len(c.Routes) == 0 {
		return errors.New("no routes selected; pick at least one game profile")
	}
	for _, r := range c.Routes {
		if r.Contains(c.Endpoint.Addr()) {
			return fmt.Errorf("route %s would capture the node endpoint itself", r)
		}
	}
	if c.MTU == 0 {
		return errors.New("mtu is required")
	}
	return nil
}

type Stats struct {
	RxBytes       int64
	TxBytes       int64
	LastHandshake time.Time
}

type Tunnel struct {
	cfg  Config
	tun  tun.Device
	dev  *device.Device
	name string
	once sync.Once
}

func Start(cfg Config) (*Tunnel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	tdev, err := createTUN(cfg.MTU)
	if err != nil {
		return nil, fmt.Errorf("create tun: %w", err)
	}
	name, err := tdev.Name()
	if err != nil {
		tdev.Close()
		return nil, err
	}
	t := &Tunnel{cfg: cfg, tun: tdev, name: name}
	t.dev = device.NewDevice(tdev, conn.NewDefaultBind(), device.NewLogger(device.LogLevelError, "tunnel: "))

	if err := t.dev.IpcSet(uapiConfig(cfg)); err != nil {
		t.dev.Close()
		return nil, fmt.Errorf("configure wireguard: %w", err)
	}
	if err := t.dev.Up(); err != nil {
		t.dev.Close()
		return nil, err
	}
	if err := configureOS(t); err != nil {
		t.dev.Close()
		return nil, fmt.Errorf("configure %s: %w", name, err)
	}
	return t, nil
}

func uapiConfig(c Config) string {
	var b strings.Builder
	fmt.Fprintf(&b, "private_key=%s\n", hex.EncodeToString(c.PrivateKey[:]))
	b.WriteString("replace_peers=true\n")
	fmt.Fprintf(&b, "public_key=%s\n", hex.EncodeToString(c.ServerPublicKey[:]))
	fmt.Fprintf(&b, "endpoint=%s\n", c.Endpoint)
	b.WriteString("persistent_keepalive_interval=25\n")
	b.WriteString("replace_allowed_ips=true\n")
	fmt.Fprintf(&b, "allowed_ip=%s\n", netip.PrefixFrom(c.Gateway, 32))
	for _, r := range c.Routes {
		fmt.Fprintf(&b, "allowed_ip=%s\n", r)
	}
	return b.String()
}

func (t *Tunnel) Name() string { return t.name }

func (t *Tunnel) RouteCount() int { return len(t.cfg.Routes) }

func (t *Tunnel) Stats() Stats {
	s, err := t.dev.IpcGet()
	if err != nil {
		return Stats{}
	}
	return parseStats(s)
}

func parseStats(s string) Stats {
	var st Stats
	var sec, nsec int64
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		n, _ := strconv.ParseInt(v, 10, 64)
		switch k {
		case "rx_bytes":
			st.RxBytes += n
		case "tx_bytes":
			st.TxBytes += n
		case "last_handshake_time_sec":
			sec = n
		case "last_handshake_time_nsec":
			nsec = n
		}
	}
	if sec > 0 {
		st.LastHandshake = time.Unix(sec, nsec)
	}
	return st
}

// Close tears down the device. Removing the interface also removes every
// route pointing at it, so nothing leaks into the OS routing table.
func (t *Tunnel) Close() {
	t.once.Do(func() {
		unconfigureOS(t)
		t.dev.Close()
	})
}
