// Package node wires together the node daemon: WireGuard, firewall, the
// pairing/control API, the admin socket and the UDP latency probe.
package node

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"

	"github.com/AltairCA/ExitLagFree/internal/netutil"
	"github.com/AltairCA/ExitLagFree/internal/proto"
)

const (
	DefaultConfigPath  = "/etc/exitlag-node/config.json"
	DefaultDataDir     = "/var/lib/exitlag-node"
	DefaultAdminSocket = "/run/exitlag-node/admin.sock"
)

type Config struct {
	NodeName   string `json:"node_name"`
	PublicHost string `json:"public_host"`
	APIPort    int    `json:"api_port"`
	WGPort     int    `json:"wg_port"`
	ProbePort  int    `json:"probe_port"`
	Interface  string `json:"interface"`
	Subnet     string `json:"subnet"`
	MTU        int    `json:"mtu"`
	MaxDevices int    `json:"max_devices"`
	// EgressAllowlist, when non-empty, restricts what peers can reach to
	// these IPv4 CIDRs (e.g. only game server ranges).
	EgressAllowlist []string `json:"egress_allowlist,omitempty"`
	DataDir         string   `json:"data_dir"`
	AdminSocket     string   `json:"admin_socket"`
}

func DefaultConfig() Config {
	host, _ := os.Hostname()
	return Config{
		NodeName:    host,
		APIPort:     proto.DefaultAPIPort,
		WGPort:      proto.DefaultWGPort,
		ProbePort:   proto.DefaultProbePort,
		Interface:   "wg-elf",
		Subnet:      "10.66.0.0/24",
		MTU:         proto.DefaultMTU,
		MaxDevices:  10,
		DataDir:     DefaultDataDir,
		AdminSocket: DefaultAdminSocket,
	}
}

func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, fmt.Errorf("config %s not found; run `exitlag-node init --public-host <ip-or-domain>` first", path)
		}
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, cfg.Validate()
}

func SaveConfig(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

func (c Config) Validate() error {
	if c.PublicHost == "" {
		return errors.New("public_host must be set to the VPS public IP or domain")
	}
	for name, p := range map[string]int{"api_port": c.APIPort, "wg_port": c.WGPort, "probe_port": c.ProbePort} {
		if p < 1 || p > 65535 {
			return fmt.Errorf("%s out of range", name)
		}
	}
	if c.WGPort == c.ProbePort {
		return errors.New("wg_port and probe_port must differ")
	}
	sub, err := c.SubnetPrefix()
	if err != nil {
		return err
	}
	if sub.Bits() > 29 {
		return errors.New("subnet must be /29 or larger")
	}
	if c.MaxDevices < 1 {
		return errors.New("max_devices must be at least 1")
	}
	if c.MTU < 1280 || c.MTU > 1500 {
		return errors.New("mtu must be between 1280 and 1500")
	}
	if _, err := netutil.ParsePrefixes(c.EgressAllowlist); err != nil {
		return fmt.Errorf("egress_allowlist: %w", err)
	}
	return nil
}

func (c Config) SubnetPrefix() (netip.Prefix, error) {
	p, err := netip.ParsePrefix(c.Subnet)
	if err != nil || !p.Addr().Is4() {
		return netip.Prefix{}, fmt.Errorf("subnet %q must be an IPv4 CIDR", c.Subnet)
	}
	return p.Masked(), nil
}

// Gateway is the node's own address inside the tunnel subnet (first host).
func (c Config) Gateway() netip.Addr {
	p, _ := c.SubnetPrefix()
	return p.Addr().Next()
}

func (c Config) Allowlist() []netip.Prefix {
	ps, _ := netutil.ParsePrefixes(c.EgressAllowlist)
	return netutil.Merge(ps)
}
