package profiles

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AltairCA/ExitLagFree/internal/netutil"
)

func TestAllProfilesValid(t *testing.T) {
	ps, err := All()
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, p := range ps {
		if p.ID == "" || p.Name == "" || ids[p.ID] {
			t.Fatalf("bad or duplicate profile %+v", p)
		}
		ids[p.ID] = true
		if len(p.CIDRs) == 0 && p.Cloud == nil {
			t.Fatalf("%s has no routes", p.ID)
		}
		if _, err := ValidateCustom(p.CIDRs); err != nil {
			t.Fatalf("%s: %v", p.ID, err)
		}
	}
	for _, want := range []string{"cs2", "valorant", "fortnite", "league", "apex"} {
		if !ids[want] {
			t.Errorf("missing profile %s", want)
		}
	}
}

func TestResolveWithAWSAndCustom(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"prefixes":[
			{"ip_prefix":"3.80.0.0/12","region":"us-east-1","service":"EC2"},
			{"ip_prefix":"3.80.0.0/12","region":"us-east-1","service":"AMAZON"},
			{"ip_prefix":"13.48.0.0/15","region":"eu-north-1","service":"EC2"}
		]}`))
	}))
	defer srv.Close()
	r := &Resolver{CacheDir: t.TempDir(), AWSRangesURL: srv.URL}
	routes, err := r.Resolve(context.Background(), Selection{
		Profiles: []string{"fortnite", "cs2"},
		Regions:  map[string][]string{"fortnite": {"aws:us-east-1"}},
		Custom:   []string{"1.2.3.4"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, s := range netutil.Strings(routes) {
		got[s] = true
	}
	for _, want := range []string{"3.80.0.0/12", "155.133.224.0/19", "1.2.3.4/32"} {
		if !got[want] {
			t.Errorf("missing %s in %v", want, routes)
		}
	}
	if got["13.48.0.0/15"] {
		t.Error("unselected region included")
	}
}

func TestResolveApexGCP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"prefixes":[
			{"ipv4Prefix":"34.80.0.0/15","service":"Google Cloud","scope":"asia-east1"},
			{"ipv6Prefix":"2600:1900:4030::/44","service":"Google Cloud","scope":"asia-east1"},
			{"ipv4Prefix":"35.184.0.0/13","service":"Google Cloud","scope":"us-central1"}
		]}`))
	}))
	defer srv.Close()
	r := &Resolver{CacheDir: t.TempDir(), GCPRangesURL: srv.URL}

	routes, err := r.Resolve(context.Background(), Selection{
		Profiles: []string{"apex"},
		Regions:  map[string][]string{"apex": {"gcp:asia-east1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, s := range netutil.Strings(routes) {
		got[s] = true
	}
	if !got["34.80.0.0/15"] || !got["5.200.0.0/19"] {
		t.Errorf("missing GCP or i3D range in %v", routes)
	}
	if got["35.184.0.0/13"] {
		t.Error("unselected region included")
	}

	if _, err := r.Resolve(context.Background(), Selection{Profiles: []string{"apex"}}); err != nil {
		t.Errorf("apex without regions should fall back to static ranges: %v", err)
	}
	if _, err := r.Resolve(context.Background(), Selection{
		Profiles: []string{"apex"},
		Regions:  map[string][]string{"apex": {"gcp:europe-north1"}},
	}); err == nil {
		t.Error("region not offered by the profile was accepted")
	}
}

func TestFortniteNeedsRegion(t *testing.T) {
	r := &Resolver{CacheDir: t.TempDir()}
	if _, err := r.Resolve(context.Background(), Selection{Profiles: []string{"fortnite"}}); err == nil {
		t.Fatal("expected error without region")
	}
}

func TestValidateCustom(t *testing.T) {
	for _, bad := range []string{"192.168.1.1", "10.0.0.0/8", "1.0.0.0/4", "2001:db8::1", "nonsense"} {
		if _, err := ValidateCustom([]string{bad}); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	if _, err := ValidateCustom([]string{"8.8.8.8", "1.1.1.0/24", "# comment", ""}); err != nil {
		t.Fatal(err)
	}
}
