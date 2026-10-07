package tunnel

import (
	"fmt"
	"net"
	"net/netip"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/tun"
)

const ifName = "elf0"

func createTUN(mtu int) (tun.Device, error) {
	return tun.CreateTUN(ifName, mtu)
}

func ipNet(p netip.Prefix) *net.IPNet {
	return &net.IPNet{IP: p.Addr().AsSlice(), Mask: net.CIDRMask(p.Bits(), p.Addr().BitLen())}
}

func configureOS(t *Tunnel) error {
	link, err := netlink.LinkByName(t.name)
	if err != nil {
		return err
	}
	addr := &netlink.Addr{IPNet: ipNet(t.cfg.Address), Peer: ipNet(netip.PrefixFrom(t.cfg.Gateway, 32))}
	if err := netlink.AddrReplace(link, addr); err != nil {
		return fmt.Errorf("assign address: %w", err)
	}
	if err := netlink.LinkSetMTU(link, t.cfg.MTU); err != nil {
		return err
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return err
	}
	for _, r := range t.cfg.Routes {
		route := &netlink.Route{LinkIndex: link.Attrs().Index, Dst: ipNet(r), Scope: netlink.SCOPE_LINK}
		if err := netlink.RouteReplace(route); err != nil {
			return fmt.Errorf("route %s: %w", r, err)
		}
	}
	return nil
}

func unconfigureOS(*Tunnel) {}
