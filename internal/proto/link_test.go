package proto

import (
	"strings"
	"testing"
)

func TestPairingLinkRoundTrip(t *testing.T) {
	in := PairingLink{
		Host:        "203.0.113.7",
		Port:        8443,
		Token:       "abc_DEF-123",
		Fingerprint: strings.Repeat("ab", 32),
		NodeName:    "sydney vps",
	}
	out, err := ParsePairingLink(in.String())
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("got %+v want %+v", out, in)
	}
	if got := out.APIBase(); got != "https://203.0.113.7:8443" {
		t.Fatalf("APIBase = %s", got)
	}
}

func TestPairingLinkIPv6(t *testing.T) {
	in := PairingLink{Host: "2001:db8::1", Port: 9000, Token: "x", Fingerprint: strings.Repeat("00", 32)}
	out, err := ParsePairingLink(in.String())
	if err != nil {
		t.Fatal(err)
	}
	if out.Host != "2001:db8::1" || out.Port != 9000 {
		t.Fatalf("got %+v", out)
	}
}

func TestPairingLinkRejects(t *testing.T) {
	fp := strings.Repeat("ab", 32)
	for _, s := range []string{
		"https://host:1?t=x&fp=" + fp,
		"elf://host:1?fp=" + fp,
		"elf://host:1?t=x&fp=zz",
		"elf://host:1?t=x&fp=abab",
		"elf://:1?t=x&fp=" + fp,
	} {
		if _, err := ParsePairingLink(s); err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
}
