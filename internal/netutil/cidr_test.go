package netutil

import (
	"net/netip"
	"reflect"
	"testing"
)

func TestMerge(t *testing.T) {
	in, _ := ParsePrefixes([]string{
		"10.0.1.0/24", "10.0.0.0/24", "10.0.0.5", "10.0.2.0/23", "192.168.1.0/24", "192.168.1.0/24",
	})
	got := Strings(Merge(in))
	want := []string{"10.0.0.0/22", "192.168.1.0/24"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestExclude(t *testing.T) {
	in, _ := ParsePrefixes([]string{"10.0.0.0/30", "1.1.1.0/24"})
	got := Strings(Exclude(in, netip.MustParseAddr("10.0.0.2")))
	want := []string{"1.1.1.0/24", "10.0.0.0/31", "10.0.0.3/32"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for _, p := range Exclude(in, netip.MustParseAddr("10.0.0.2")) {
		if p.Contains(netip.MustParseAddr("10.0.0.2")) {
			t.Fatalf("%v still contains excluded addr", p)
		}
	}
}

func TestParseRejectsIPv6(t *testing.T) {
	if _, err := ParsePrefixes([]string{"2001:db8::/32"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestIsPublicIPv4(t *testing.T) {
	cases := map[string]bool{
		"8.8.8.8":     true,
		"10.1.2.3":    false,
		"100.64.1.1":  false,
		"127.0.0.1":   false,
		"169.254.1.1": false,
		"224.0.0.1":   false,
		"::1":         false,
	}
	for s, want := range cases {
		if got := IsPublicIPv4(netip.MustParseAddr(s)); got != want {
			t.Errorf("%s: got %v want %v", s, got, want)
		}
	}
}
