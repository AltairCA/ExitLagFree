//go:build linux

package wg

import (
	"errors"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/ipc"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type linuxManager struct {
	opts    Options
	client  *wgctrl.Client
	backend string

	mu   sync.Mutex
	dev  *device.Device
	uapi net.Listener
}

func Setup(opts Options) (Manager, error) {
	if old, err := netlink.LinkByName(opts.Interface); err == nil {
		_ = netlink.LinkDel(old)
	}
	m := &linuxManager{opts: opts}

	attrs := netlink.NewLinkAttrs()
	attrs.Name = opts.Interface
	attrs.MTU = opts.MTU
	if err := netlink.LinkAdd(&netlink.Wireguard{LinkAttrs: attrs}); err == nil {
		m.backend = "kernel"
	} else {
		log.Printf("wg: kernel WireGuard unavailable (%v), using userspace wireguard-go", err)
		if err := m.startUserspace(); err != nil {
			return nil, err
		}
		m.backend = "userspace"
	}

	link, err := netlink.LinkByName(opts.Interface)
	if err != nil {
		m.Close()
		return nil, err
	}
	addr := &netlink.Addr{IPNet: &net.IPNet{
		IP:   opts.Address.Addr().AsSlice(),
		Mask: net.CIDRMask(opts.Address.Bits(), 32),
	}}
	if err := netlink.AddrReplace(link, addr); err != nil {
		m.Close()
		return nil, fmt.Errorf("assign %s: %w", opts.Address, err)
	}
	if err := netlink.LinkSetMTU(link, opts.MTU); err != nil {
		log.Printf("wg: set mtu: %v", err)
	}

	m.client, err = wgctrl.New()
	if err != nil {
		m.Close()
		return nil, err
	}
	port := opts.ListenPort
	if err := m.client.ConfigureDevice(opts.Interface, wgtypes.Config{
		PrivateKey:   &opts.PrivateKey,
		ListenPort:   &port,
		ReplacePeers: true,
	}); err != nil {
		m.Close()
		return nil, fmt.Errorf("configure %s: %w", opts.Interface, err)
	}
	if err := netlink.LinkSetUp(link); err != nil {
		m.Close()
		return nil, err
	}
	return m, nil
}

func (m *linuxManager) startUserspace() error {
	tdev, err := tun.CreateTUN(m.opts.Interface, m.opts.MTU)
	if err != nil {
		return fmt.Errorf("create tun: %w", err)
	}
	m.dev = device.NewDevice(tdev, conn.NewDefaultBind(), device.NewLogger(device.LogLevelError, "wg: "))
	f, err := ipc.UAPIOpen(m.opts.Interface)
	if err != nil {
		m.dev.Close()
		return fmt.Errorf("uapi open: %w", err)
	}
	m.uapi, err = ipc.UAPIListen(m.opts.Interface, f)
	if err != nil {
		m.dev.Close()
		return fmt.Errorf("uapi listen: %w", err)
	}
	go func() {
		for {
			c, err := m.uapi.Accept()
			if err != nil {
				return
			}
			go m.dev.IpcHandle(c)
		}
	}()
	return m.dev.Up()
}

func (m *linuxManager) Backend() string { return m.backend }

func (m *linuxManager) Sync(peers []Peer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, err := m.client.Device(m.opts.Interface)
	if err != nil {
		return err
	}
	want := make(map[wgtypes.Key]Peer, len(peers))
	for _, p := range peers {
		want[p.PublicKey] = p
	}
	var cfgs []wgtypes.PeerConfig
	for _, p := range cur.Peers {
		w, ok := want[p.PublicKey]
		if !ok {
			cfgs = append(cfgs, wgtypes.PeerConfig{PublicKey: p.PublicKey, Remove: true})
			continue
		}
		if len(p.AllowedIPs) == 1 && p.AllowedIPs[0].IP.Equal(w.Address.AsSlice()) {
			delete(want, p.PublicKey)
		}
	}
	for _, p := range want {
		cfgs = append(cfgs, wgtypes.PeerConfig{
			PublicKey:         p.PublicKey,
			ReplaceAllowedIPs: true,
			AllowedIPs: []net.IPNet{{
				IP:   p.Address.AsSlice(),
				Mask: net.CIDRMask(32, 32),
			}},
		})
	}
	if len(cfgs) == 0 {
		return nil
	}
	return m.client.ConfigureDevice(m.opts.Interface, wgtypes.Config{Peers: cfgs})
}

func (m *linuxManager) Stats() (map[wgtypes.Key]PeerStats, error) {
	d, err := m.client.Device(m.opts.Interface)
	if err != nil {
		return nil, err
	}
	out := make(map[wgtypes.Key]PeerStats, len(d.Peers))
	for _, p := range d.Peers {
		s := PeerStats{
			LastHandshake: p.LastHandshakeTime,
			RxBytes:       p.ReceiveBytes,
			TxBytes:       p.TransmitBytes,
		}
		if p.Endpoint != nil {
			s.Endpoint = p.Endpoint.String()
		}
		out[p.PublicKey] = s
	}
	return out, nil
}

func (m *linuxManager) Close() error {
	var errs []error
	if m.client != nil {
		errs = append(errs, m.client.Close())
	}
	if m.uapi != nil {
		errs = append(errs, m.uapi.Close())
	}
	if m.dev != nil {
		m.dev.Close()
	}
	if link, err := netlink.LinkByName(m.opts.Interface); err == nil {
		errs = append(errs, netlink.LinkDel(link))
	}
	return errors.Join(errs...)
}
