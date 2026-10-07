package firewall

import (
	"net/netip"
	"strings"
	"testing"
)

func testRules(allow ...string) Rules {
	r := Rules{
		Interface: "wg-elf",
		Subnet:    netip.MustParsePrefix("10.66.0.0/24"),
		Gateway:   netip.MustParseAddr("10.66.0.1"),
	}
	for _, a := range allow {
		r.EgressAllowlist = append(r.EgressAllowlist, netip.MustParsePrefix(a))
	}
	return r
}

func TestNFTRuleset(t *testing.T) {
	s := testRules().NFTRuleset()
	for _, want := range []string{
		`delete table inet exitlagfree`,
		`iifname "wg-elf" oifname "wg-elf" drop`,
		`iifname "wg-elf" ip daddr 10.66.0.1 icmp type echo-request accept`,
		`iifname "wg-elf" drop`,
		`10.0.0.0/8`,
		`192.168.0.0/16`,
		`ip saddr 10.66.0.0/24 oifname != "wg-elf" masquerade`,
		`ip saddr != 10.66.0.0/24 drop`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("ruleset missing %q\n%s", want, s)
		}
	}
	if strings.Contains(s, "ip daddr != {") {
		t.Error("allowlist rule present without allowlist")
	}
}

func TestNFTRulesetAllowlist(t *testing.T) {
	s := testRules("155.133.224.0/19", "104.160.128.0/19").NFTRuleset()
	if !strings.Contains(s, `ip daddr != { 155.133.224.0/19, 104.160.128.0/19 } drop`) {
		t.Errorf("allowlist rule missing:\n%s", s)
	}
}

func TestIPTablesSetupAllowlist(t *testing.T) {
	var joined []string
	for _, c := range testRules("1.2.3.0/24").IPTablesSetup() {
		joined = append(joined, strings.Join(c, " "))
	}
	all := strings.Join(joined, "\n")
	for _, want := range []string{
		"-A ELF-EGRESS -d 1.2.3.0/24 -j RETURN",
		"-A ELF-EGRESS -j DROP",
		"-t nat -A ELF-POST -s 10.66.0.0/24 ! -o wg-elf -j MASQUERADE",
		"-A ELF-FWD -i wg-elf -o wg-elf -j DROP",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q", want)
		}
	}
}
