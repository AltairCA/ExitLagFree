// Package proto holds the wire types shared between the node daemon, the
// privileged client helper and the desktop app.
package proto

import "time"

const (
	// APIVersion is bumped on breaking changes to the node control API.
	APIVersion = 1

	DefaultAPIPort   = 8443
	DefaultWGPort    = 51820
	DefaultProbePort = 51821
	DefaultMTU       = 1420
)

// PairRequest is sent by a client that holds a one-time invite token.
// The private key never leaves the device; only the public key is sent.
type PairRequest struct {
	Token     string `json:"token"`
	PublicKey string `json:"public_key"`
}

// PairResponse contains everything the client needs to bring up a tunnel.
type PairResponse struct {
	DeviceID        string `json:"device_id"`
	DeviceName      string `json:"device_name"`
	DeviceToken     string `json:"device_token"`
	Address         string `json:"address"`
	Gateway         string `json:"gateway"`
	ServerPublicKey string `json:"server_public_key"`
	Endpoint        string `json:"endpoint"`
	ProbePort       int    `json:"probe_port"`
	MTU             int    `json:"mtu"`
	NodeName        string `json:"node_name"`
}

// MeResponse describes the calling device as seen by the node.
type MeResponse struct {
	DeviceID      string    `json:"device_id"`
	DeviceName    string    `json:"device_name"`
	Address       string    `json:"address"`
	LastHandshake time.Time `json:"last_handshake,omitempty"`
	NodeName      string    `json:"node_name"`
	NodeVersion   string    `json:"node_version"`
}

// ProbeResponse is the node's measured latency to a target host.
type ProbeResponse struct {
	Target   string  `json:"target"`
	RTTMs    float64 `json:"rtt_ms"`
	LossPct  float64 `json:"loss_pct"`
	Received int     `json:"received"`
	Sent     int     `json:"sent"`
}

// ErrorResponse is returned with any non-2xx status.
type ErrorResponse struct {
	Error string `json:"error"`
}

// Admin API (local unix socket on the node, root only).

type InviteRequest struct {
	Name string        `json:"name"`
	TTL  time.Duration `json:"ttl"`
}

type InviteResponse struct {
	Link      string    `json:"link"`
	ExpiresAt time.Time `json:"expires_at"`
}

type DeviceInfo struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Address       string    `json:"address"`
	PublicKey     string    `json:"public_key"`
	CreatedAt     time.Time `json:"created_at"`
	LastHandshake time.Time `json:"last_handshake,omitempty"`
	RxBytes       int64     `json:"rx_bytes"`
	TxBytes       int64     `json:"tx_bytes"`
	Endpoint      string    `json:"endpoint,omitempty"`
}

type NodeStatus struct {
	NodeName       string `json:"node_name"`
	Version        string `json:"version"`
	PublicHost     string `json:"public_host"`
	Interface      string `json:"interface"`
	WGBackend      string `json:"wg_backend"`
	Firewall       string `json:"firewall"`
	Devices        int    `json:"devices"`
	MaxDevices     int    `json:"max_devices"`
	PendingInvites int    `json:"pending_invites"`
	Fingerprint    string `json:"fingerprint"`
}
