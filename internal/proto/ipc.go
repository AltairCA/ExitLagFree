package proto

import "time"

// Types exchanged between the unprivileged desktop app and the privileged
// exitlag-helper service over a local socket / named pipe.

type TunnelState string

const (
	StateDisconnected TunnelState = "disconnected"
	StateConnecting   TunnelState = "connecting"
	StateConnected    TunnelState = "connected"
	StateError        TunnelState = "error"
)

// ConnectRequest carries a full tunnel configuration. The helper keeps the
// private key only in memory for the lifetime of the tunnel.
type ConnectRequest struct {
	NodeID          string   `json:"node_id"`
	NodeName        string   `json:"node_name"`
	PrivateKey      string   `json:"private_key"`
	Address         string   `json:"address"`
	Gateway         string   `json:"gateway"`
	ServerPublicKey string   `json:"server_public_key"`
	Endpoint        string   `json:"endpoint"`
	MTU             int      `json:"mtu"`
	Routes          []string `json:"routes"`
}

type HelperStatus struct {
	Version       string      `json:"version"`
	State         TunnelState `json:"state"`
	Error         string      `json:"error,omitempty"`
	NodeID        string      `json:"node_id,omitempty"`
	NodeName      string      `json:"node_name,omitempty"`
	Interface     string      `json:"interface,omitempty"`
	ConnectedAt   time.Time   `json:"connected_at,omitempty"`
	LastHandshake time.Time   `json:"last_handshake,omitempty"`
	RxBytes       int64       `json:"rx_bytes"`
	TxBytes       int64       `json:"tx_bytes"`
	Routes        int         `json:"routes"`
}

type PingRequest struct {
	Targets []string      `json:"targets"`
	Count   int           `json:"count"`
	Timeout time.Duration `json:"timeout"`
}

type PingResult struct {
	Target  string  `json:"target"`
	RTTMs   float64 `json:"rtt_ms"`
	LossPct float64 `json:"loss_pct"`
	Error   string  `json:"error,omitempty"`
}

type PingResponse struct {
	Results []PingResult `json:"results"`
}
