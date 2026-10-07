package tunnel

import (
	"bytes"
	"fmt"
	"net/netip"
	"os/exec"
	"strconv"

	"golang.zx2c4.com/wireguard/tun"
)

func createTUN(mtu int) (tun.Device, error) {
	// "utun" lets the kernel pick the next free utunN.
	return tun.CreateTUN("utun", mtu)
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %v: %s", name, args, err, bytes.TrimSpace(out))
	}
	return nil
}

func configureOS(t *Tunnel) error {
	addr := t.cfg.Address.Addr().String()
	if err := run("/sbin/ifconfig", t.name, "inet", addr, t.cfg.Gateway.String(),
		"netmask", "255.255.255.255", "mtu", strconv.Itoa(t.cfg.MTU), "up"); err != nil {
		return err
	}
	for _, r := range t.cfg.Routes {
		if err := addRoute(r, t.name); err != nil {
			return err
		}
	}
	return nil
}

func addRoute(p netip.Prefix, iface string) error {
	return run("/sbin/route", "-q", "-n", "add", "-inet", p.String(), "-interface", iface)
}

// Routes bound to a utun interface are removed by the kernel when the
// interface is destroyed, which happens when the wireguard device closes.
func unconfigureOS(*Tunnel) {}
