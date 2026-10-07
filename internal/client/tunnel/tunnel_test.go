package tunnel

import (
	"net/netip"
	"strings"
	"testing"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestUAPIConfig(t *testing.T) {
	priv, _ := wgtypes.GeneratePrivateKey()
	c := Config{
		PrivateKey:      priv,
		ServerPublicKey: priv.PublicKey(),
		Address:         netip.MustParsePrefix("10.66.0.5/32"),
		Gateway:         netip.MustParseAddr("10.66.0.1"),
		Endpoint:        netip.MustParseAddrPort("203.0.113.9:51820"),
		MTU:             1420,
		Routes:          []netip.Prefix{netip.MustParsePrefix("155.133.224.0/19")},
	}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	s := uapiConfig(c)
	for _, want := range []string{
		"endpoint=203.0.113.9:51820",
		"allowed_ip=10.66.0.1/32",
		"allowed_ip=155.133.224.0/19",
		"persistent_keepalive_interval=25",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}

	c.Routes = append(c.Routes, netip.MustParsePrefix("203.0.113.0/24"))
	if err := c.validate(); err == nil {
		t.Fatal("expected error for route covering endpoint")
	}
}

func TestParseStats(t *testing.T) {
	st := parseStats("public_key=abc\nrx_bytes=100\ntx_bytes=50\nlast_handshake_time_sec=1700000000\nlast_handshake_time_nsec=5\n")
	if st.RxBytes != 100 || st.TxBytes != 50 || st.LastHandshake.Unix() != 1700000000 {
		t.Fatalf("got %+v", st)
	}
}
