package tunnel

import (
	"errors"
	"fmt"
	"net/netip"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

const ifName = "ExitLagFree"

// A fixed adapter GUID keeps Windows from creating a new "Network N"
// profile every time the tunnel comes up.
var adapterGUID = windows.GUID{Data1: 0x6c1f3a52, Data2: 0x8e0b, Data3: 0x4d7a, Data4: [8]byte{0x9a, 0x51, 0x2e, 0x10, 0x4b, 0x7c, 0xe1, 0x0f}}

func createTUN(mtu int) (tun.Device, error) {
	tun.WintunTunnelType = "ExitLagFree"
	return tun.CreateTUNWithRequestedGUID(ifName, &adapterGUID, mtu)
}

func configureOS(t *Tunnel) error {
	nt, ok := t.tun.(*tun.NativeTun)
	if !ok {
		return errors.New("unexpected tun implementation")
	}
	luid := winipcfg.LUID(nt.LUID())
	if err := luid.SetIPAddressesForFamily(windows.AF_INET, []netip.Prefix{t.cfg.Address}); err != nil {
		return fmt.Errorf("assign address: %w", err)
	}
	routes := make([]*winipcfg.RouteData, 0, len(t.cfg.Routes)+1)
	routes = append(routes, &winipcfg.RouteData{Destination: netip.PrefixFrom(t.cfg.Gateway, 32), NextHop: netip.IPv4Unspecified()})
	for _, r := range t.cfg.Routes {
		routes = append(routes, &winipcfg.RouteData{Destination: r, NextHop: netip.IPv4Unspecified()})
	}
	if err := luid.SetRoutesForFamily(windows.AF_INET, routes); err != nil {
		return fmt.Errorf("set routes: %w", err)
	}
	iface, err := luid.IPInterface(windows.AF_INET)
	if err != nil {
		return err
	}
	iface.NLMTU = uint32(t.cfg.MTU)
	iface.UseAutomaticMetric = false
	iface.Metric = 0
	iface.DadTransmits = 0
	iface.RouterDiscoveryBehavior = winipcfg.RouterDiscoveryDisabled
	return iface.Set()
}

func unconfigureOS(t *Tunnel) {
	if nt, ok := t.tun.(*tun.NativeTun); ok {
		luid := winipcfg.LUID(nt.LUID())
		_ = luid.FlushRoutes(windows.AF_INET)
		_ = luid.FlushIPAddresses(windows.AF_INET)
	}
}
