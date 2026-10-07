// Package netutil contains IPv4 prefix helpers used to build split-tunnel route sets.
package netutil

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

// ParsePrefixes accepts CIDRs or bare IPv4 addresses (treated as /32).
// IPv6 entries are rejected because the tunnel is IPv4-only.
func ParsePrefixes(items []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(items))
	for _, raw := range items {
		s := strings.TrimSpace(raw)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		var p netip.Prefix
		if strings.Contains(s, "/") {
			var err error
			p, err = netip.ParsePrefix(s)
			if err != nil {
				return nil, fmt.Errorf("invalid CIDR %q: %w", s, err)
			}
		} else {
			a, err := netip.ParseAddr(s)
			if err != nil {
				return nil, fmt.Errorf("invalid IP %q: %w", s, err)
			}
			p = netip.PrefixFrom(a, a.BitLen())
		}
		if !p.Addr().Is4() {
			return nil, fmt.Errorf("%q: only IPv4 is supported", s)
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

// Merge sorts, de-duplicates and collapses prefixes so the OS gets the
// smallest equivalent route table.
func Merge(in []netip.Prefix) []netip.Prefix {
	if len(in) == 0 {
		return nil
	}
	ps := make([]netip.Prefix, len(in))
	for i, p := range in {
		ps[i] = p.Masked()
	}
	sort.Slice(ps, func(i, j int) bool {
		if c := ps[i].Addr().Compare(ps[j].Addr()); c != 0 {
			return c < 0
		}
		return ps[i].Bits() < ps[j].Bits()
	})
	out := ps[:0:0]
	for _, p := range ps {
		if n := len(out); n > 0 && out[n-1].Contains(p.Addr()) && out[n-1].Bits() <= p.Bits() {
			continue
		}
		out = append(out, p)
		for len(out) >= 2 {
			a, b := out[len(out)-2], out[len(out)-1]
			if a.Bits() != b.Bits() || a.Bits() == 0 {
				break
			}
			parent := netip.PrefixFrom(a.Addr(), a.Bits()-1).Masked()
			if parent.Addr() != a.Addr() || !parent.Contains(b.Addr()) {
				break
			}
			out = append(out[:len(out)-2], parent)
		}
	}
	return out
}

// Exclude removes addr from the prefix set by splitting any prefix that
// contains it. Used to keep the VPS endpoint itself out of the tunnel.
func Exclude(in []netip.Prefix, addr netip.Addr) []netip.Prefix {
	var out []netip.Prefix
	for _, p := range in {
		if !p.Contains(addr) {
			out = append(out, p)
			continue
		}
		cur := p
		for cur.Bits() < addr.BitLen() {
			lo := netip.PrefixFrom(cur.Addr(), cur.Bits()+1)
			hi := netip.PrefixFrom(lastAddr(lo).Next(), cur.Bits()+1)
			if lo.Contains(addr) {
				out = append(out, hi)
				cur = lo
			} else {
				out = append(out, lo)
				cur = hi
			}
		}
	}
	return Merge(out)
}

// IsPublicIPv4 reports whether a is a globally routable unicast IPv4 address.
func IsPublicIPv4(a netip.Addr) bool {
	if !a.Is4() || !a.IsGlobalUnicast() || a.IsPrivate() {
		return false
	}
	for _, p := range NonPublicV4 {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

// NonPublicV4 lists ranges a node must never forward client traffic to.
var NonPublicV4 = mustPrefixes(
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"192.168.0.0/16",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"224.0.0.0/4",
	"240.0.0.0/4",
)

func lastAddr(p netip.Prefix) netip.Addr {
	a := p.Masked().Addr().As4()
	hostBits := 32 - p.Bits()
	v := uint32(a[0])<<24 | uint32(a[1])<<16 | uint32(a[2])<<8 | uint32(a[3])
	if hostBits > 0 {
		v |= (1 << hostBits) - 1
	}
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

func mustPrefixes(s ...string) []netip.Prefix {
	ps, err := ParsePrefixes(s)
	if err != nil {
		panic(err)
	}
	return ps
}

func Strings(ps []netip.Prefix) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.String()
	}
	return out
}
