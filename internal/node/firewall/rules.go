// Package firewall installs the node's NAT and filtering rules.
//
// Policy for traffic arriving from the WireGuard interface:
//   - peers may not talk to each other or to the VPS host itself (except
//     ICMP echo to the tunnel gateway, used for in-tunnel latency checks)
//   - peers may not reach private, loopback, link-local, CGNAT or multicast
//     ranges, so a node can't be used to pivot into the provider's network
//   - source addresses must belong to the tunnel subnet
//   - if an egress allowlist is configured, only those ranges are reachable
//   - everything else is masqueraded out of the default interface
package firewall

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/AltairCA/ExitLagFree/internal/netutil"
)

const (
	tableName = "exitlagfree"
	chainIn   = "ELF-IN"
	chainFwd  = "ELF-FWD"
	chainEgr  = "ELF-EGRESS"
	chainPost = "ELF-POST"
)

type Rules struct {
	Interface       string
	Subnet          netip.Prefix
	Gateway         netip.Addr
	EgressAllowlist []netip.Prefix
}

func (r Rules) blocked() []string {
	return netutil.Strings(netutil.NonPublicV4)
}

// NFTRuleset renders an idempotent nftables script: it creates the table if
// missing, deletes it, and recreates it in one atomic transaction.
func (r Rules) NFTRuleset() string {
	iif := fmt.Sprintf("iifname %q", r.Interface)
	oif := fmt.Sprintf("oifname %q", r.Interface)
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }

	w("table inet %s {}", tableName)
	w("delete table inet %s", tableName)
	w("table inet %s {", tableName)

	w("  chain input {")
	w("    type filter hook input priority -5; policy accept;")
	w("    %s ct state established,related accept", iif)
	w("    %s ip daddr %s icmp type echo-request accept", iif, r.Gateway)
	w("    %s drop", iif)
	w("  }")

	w("  chain forward {")
	w("    type filter hook forward priority -5; policy accept;")
	w("    %s tcp flags & (syn|rst) == syn tcp option maxseg size set rt mtu", iif)
	w("    %s tcp flags & (syn|rst) == syn tcp option maxseg size set rt mtu", oif)
	w("    %s %s drop", iif, oif)
	w("    %s ct state established,related accept", oif)
	w("    %s drop", oif)
	w("    %s meta nfproto ipv6 drop", iif)
	w("    %s ip saddr != %s drop", iif, r.Subnet)
	w("    %s ip daddr { %s } drop", iif, strings.Join(r.blocked(), ", "))
	if len(r.EgressAllowlist) > 0 {
		w("    %s ip daddr != { %s } drop", iif, strings.Join(netutil.Strings(r.EgressAllowlist), ", "))
	}
	w("    %s accept", iif)
	w("  }")

	w("  chain postrouting {")
	w("    type nat hook postrouting priority 100; policy accept;")
	w("    ip saddr %s %s masquerade", r.Subnet, strings.Replace(oif, "oifname", "oifname !=", 1))
	w("  }")
	w("}")
	return b.String()
}

// IPTablesSetup returns iptables invocations (without the leading binary)
// that implement the same policy for hosts without nftables.
func (r Rules) IPTablesSetup() [][]string {
	i, s, g := r.Interface, r.Subnet.String(), r.Gateway.String()
	cmds := [][]string{
		{"-N", chainIn},
		{"-A", chainIn, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"},
		{"-A", chainIn, "-d", g, "-p", "icmp", "--icmp-type", "echo-request", "-j", "ACCEPT"},
		{"-A", chainIn, "-j", "DROP"},
		{"-I", "INPUT", "1", "-i", i, "-j", chainIn},

		{"-N", chainFwd},
		{"-A", chainFwd, "-i", i, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
		{"-A", chainFwd, "-o", i, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
		{"-A", chainFwd, "-i", i, "-o", i, "-j", "DROP"},
		{"-A", chainFwd, "-o", i, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"},
		{"-A", chainFwd, "-o", i, "-j", "DROP"},
		{"-A", chainFwd, "-i", i, "!", "-s", s, "-j", "DROP"},
	}
	for _, p := range r.blocked() {
		cmds = append(cmds, []string{"-A", chainFwd, "-i", i, "-d", p, "-j", "DROP"})
	}
	if len(r.EgressAllowlist) > 0 {
		cmds = append(cmds, []string{"-N", chainEgr})
		for _, p := range r.EgressAllowlist {
			cmds = append(cmds, []string{"-A", chainEgr, "-d", p.String(), "-j", "RETURN"})
		}
		cmds = append(cmds,
			[]string{"-A", chainEgr, "-j", "DROP"},
			[]string{"-A", chainFwd, "-i", i, "-j", chainEgr},
		)
	}
	cmds = append(cmds,
		[]string{"-A", chainFwd, "-i", i, "-j", "ACCEPT"},
		[]string{"-I", "FORWARD", "1", "-j", chainFwd},

		[]string{"-t", "nat", "-N", chainPost},
		[]string{"-t", "nat", "-A", chainPost, "-s", s, "!", "-o", i, "-j", "MASQUERADE"},
		[]string{"-t", "nat", "-I", "POSTROUTING", "1", "-j", chainPost},
	)
	return cmds
}

// IPTablesTeardown removes everything IPTablesSetup created. Errors are
// expected (and ignored by the caller) for parts that don't exist.
func (r Rules) IPTablesTeardown() [][]string {
	i := r.Interface
	return [][]string{
		{"-D", "INPUT", "-i", i, "-j", chainIn},
		{"-D", "FORWARD", "-j", chainFwd},
		{"-t", "nat", "-D", "POSTROUTING", "-j", chainPost},
		{"-F", chainIn}, {"-X", chainIn},
		{"-F", chainFwd}, {"-X", chainFwd},
		{"-F", chainEgr}, {"-X", chainEgr},
		{"-t", "nat", "-F", chainPost}, {"-t", "nat", "-X", chainPost},
	}
}

// CompatAccept returns rules that let our already-filtered traffic through
// legacy iptables FORWARD chains whose default policy is DROP (ufw, Docker).
// The nftables forward chain runs first at a higher priority and has
// already dropped anything disallowed.
func (r Rules) CompatAccept() [][]string {
	return [][]string{
		{"FORWARD", "-i", r.Interface, "-j", "ACCEPT"},
		{"FORWARD", "-o", r.Interface, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"},
	}
}
