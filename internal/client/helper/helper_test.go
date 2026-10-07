package helper

import (
	"context"
	"testing"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/AltairCA/ExitLagFree/internal/proto"
)

func baseReq(t *testing.T, routes ...string) proto.ConnectRequest {
	k, _ := wgtypes.GeneratePrivateKey()
	return proto.ConnectRequest{
		PrivateKey:      k.String(),
		ServerPublicKey: k.PublicKey().String(),
		Address:         "10.66.0.2/32",
		Gateway:         "10.66.0.1",
		Endpoint:        "155.133.230.10:51820",
		MTU:             1420,
		Routes:          routes,
	}
}

func TestBuildConfigExcludesEndpoint(t *testing.T) {
	cfg, err := buildConfig(context.Background(), baseReq(t, "155.133.224.0/19", "162.254.192.0/21"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range cfg.Routes {
		if r.Contains(cfg.Endpoint.Addr()) {
			t.Fatalf("route %s still contains endpoint", r)
		}
	}
	if len(cfg.Routes) < 3 {
		t.Fatalf("expected split routes, got %v", cfg.Routes)
	}
}

func TestBuildConfigRejectsPrivateRoutes(t *testing.T) {
	for _, r := range []string{"192.168.1.0/24", "10.0.0.0/8", "0.0.0.0/0"} {
		if _, err := buildConfig(context.Background(), baseReq(t, r)); err == nil {
			t.Errorf("route %s accepted", r)
		}
	}
}
