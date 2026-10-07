// Package wg manages the node's WireGuard interface. On Linux it prefers the
// kernel module and falls back to an in-process wireguard-go device, exposing
// the standard UAPI socket so wgctrl (and `wg show`) work with either.
package wg

import (
	"net/netip"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type Options struct {
	Interface  string
	PrivateKey wgtypes.Key
	ListenPort int
	Address    netip.Prefix // gateway address with subnet, e.g. 10.66.0.1/24
	MTU        int
}

type Peer struct {
	PublicKey wgtypes.Key
	Address   netip.Addr // single /32 tunnel IP owned by this peer
}

type PeerStats struct {
	LastHandshake time.Time
	RxBytes       int64
	TxBytes       int64
	Endpoint      string
}

type Manager interface {
	// Backend is "kernel" or "userspace".
	Backend() string
	// Sync makes the interface's peer list exactly match peers without
	// disturbing sessions of peers that are unchanged.
	Sync(peers []Peer) error
	Stats() (map[wgtypes.Key]PeerStats, error)
	Close() error
}
