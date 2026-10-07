// Package icmpping measures ICMP echo latency. It uses raw (privileged)
// sockets, so callers must run as root / Administrator.
package icmpping

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

type Result struct {
	RTT      time.Duration
	LossPct  float64
	Sent     int
	Received int
}

func Ping(ctx context.Context, target string, count int, timeout time.Duration) (Result, error) {
	addr, err := netip.ParseAddr(target)
	if err != nil || !addr.Is4() {
		return Result{}, fmt.Errorf("target must be an IPv4 address")
	}
	if count <= 0 {
		count = 4
	}
	if timeout <= 0 {
		timeout = time.Duration(count)*time.Second + time.Second
	}
	p, err := probing.NewPinger(addr.String())
	if err != nil {
		return Result{}, err
	}
	p.Count = count
	p.Interval = 200 * time.Millisecond
	p.Timeout = timeout
	// Unprivileged ICMP datagram sockets work on macOS (and on Linux when
	// net.ipv4.ping_group_range allows it), so non-root callers still work there.
	p.SetPrivileged(isRoot())
	if err := p.RunWithContext(ctx); err != nil {
		return Result{}, err
	}
	st := p.Statistics()
	return Result{
		RTT:      st.AvgRtt,
		LossPct:  st.PacketLoss,
		Sent:     st.PacketsSent,
		Received: st.PacketsRecv,
	}, nil
}
