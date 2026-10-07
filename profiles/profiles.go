// Package profiles ships the built-in game profiles and resolves a user's
// selection (profiles + cloud regions + custom entries) into a route set.
package profiles

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AltairCA/ExitLagFree/internal/netutil"
)

//go:embed *.json
var files embed.FS

const (
	ProviderAWS = "aws"
	ProviderGCP = "gcp"
)

// CloudRegion is a selectable region. ID is "<provider>:<region>", e.g.
// "aws:us-east-1" or "gcp:asia-northeast1".
type CloudRegion struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

func (r CloudRegion) split() (provider, region string) {
	provider, region, _ = strings.Cut(r.ID, ":")
	return provider, region
}

// CloudSource lets a profile route the published ranges of cloud regions
// where a game's servers run.
type CloudSource struct {
	// AWSServices filters Amazon's ranges (e.g. "EC2"); empty means all.
	AWSServices []string      `json:"aws_services,omitempty"`
	Regions     []CloudRegion `json:"regions"`
}

type Profile struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Notes       string       `json:"notes,omitempty"`
	CIDRs       []string     `json:"cidrs"`
	Cloud       *CloudSource `json:"cloud,omitempty"`
}

func All() ([]Profile, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, err
	}
	var out []Profile
	for _, e := range entries {
		b, err := files.ReadFile(e.Name())
		if err != nil {
			return nil, err
		}
		var p Profile
		if err := json.Unmarshal(b, &p); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if _, err := netutil.ParsePrefixes(p.CIDRs); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if p.Cloud != nil {
			for _, r := range p.Cloud.Regions {
				if prov, reg := r.split(); (prov != ProviderAWS && prov != ProviderGCP) || reg == "" {
					return nil, fmt.Errorf("%s: invalid region id %q", e.Name(), r.ID)
				}
			}
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Selection is what the user picked in the UI.
type Selection struct {
	Profiles []string            `json:"profiles"`
	Regions  map[string][]string `json:"regions,omitempty"` // profile id -> cloud region ids
	Custom   []string            `json:"custom,omitempty"`
}

// ValidateCustom checks user-entered IPs/CIDRs: IPv4, public, not too broad.
func ValidateCustom(items []string) ([]netip.Prefix, error) {
	ps, err := netutil.ParsePrefixes(items)
	if err != nil {
		return nil, err
	}
	for _, p := range ps {
		if p.Bits() < 8 {
			return nil, fmt.Errorf("%s is too broad (use /8 or narrower)", p)
		}
		for _, np := range netutil.NonPublicV4 {
			if p.Overlaps(np) {
				return nil, fmt.Errorf("%s overlaps private/reserved range %s", p, np)
			}
		}
	}
	return ps, nil
}

type Resolver struct {
	CacheDir string
	HTTP     *http.Client
	// AWSRangesURL and GCPRangesURL are overridable for tests.
	AWSRangesURL string
	GCPRangesURL string
}

const (
	awsRangesURL = "https://ip-ranges.amazonaws.com/ip-ranges.json"
	gcpRangesURL = "https://www.gstatic.com/ipranges/cloud.json"
)

// Resolve returns the merged route set for a selection.
func (r *Resolver) Resolve(ctx context.Context, sel Selection) ([]netip.Prefix, error) {
	all, err := All()
	if err != nil {
		return nil, err
	}
	byID := map[string]Profile{}
	for _, p := range all {
		byID[p.ID] = p
	}
	var out []netip.Prefix
	for _, id := range sel.Profiles {
		p, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("unknown profile %q", id)
		}
		ps, _ := netutil.ParsePrefixes(p.CIDRs)
		out = append(out, ps...)
		if p.Cloud == nil {
			continue
		}
		chosen := sel.Regions[id]
		if len(chosen) == 0 {
			if len(p.CIDRs) == 0 {
				return nil, fmt.Errorf("%s: choose at least one region", p.Name)
			}
			continue
		}
		cloud, err := r.cloudPrefixes(ctx, p.Cloud, chosen)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.Name, err)
		}
		out = append(out, cloud...)
	}
	custom, err := ValidateCustom(sel.Custom)
	if err != nil {
		return nil, err
	}
	out = append(out, custom...)
	return netutil.Merge(out), nil
}

func (r *Resolver) cloudPrefixes(ctx context.Context, src *CloudSource, chosen []string) ([]netip.Prefix, error) {
	allowed := map[string]CloudRegion{}
	for _, reg := range src.Regions {
		allowed[reg.ID] = reg
	}
	want := map[string]map[string]bool{}
	for _, id := range chosen {
		reg, ok := allowed[id]
		if !ok {
			return nil, fmt.Errorf("region %q is not offered by this profile", id)
		}
		prov, name := reg.split()
		if want[prov] == nil {
			want[prov] = map[string]bool{}
		}
		want[prov][name] = true
	}
	var out []netip.Prefix
	if regions := want[ProviderAWS]; regions != nil {
		ps, err := r.awsPrefixes(ctx, src.AWSServices, regions)
		if err != nil {
			return nil, err
		}
		out = append(out, ps...)
	}
	if regions := want[ProviderGCP]; regions != nil {
		ps, err := r.gcpPrefixes(ctx, regions)
		if err != nil {
			return nil, err
		}
		out = append(out, ps...)
	}
	if len(out) == 0 {
		return nil, errors.New("no published ranges matched the selected regions")
	}
	return out, nil
}

func (r *Resolver) awsPrefixes(ctx context.Context, services []string, regions map[string]bool) ([]netip.Prefix, error) {
	url := r.AWSRangesURL
	if url == "" {
		url = awsRangesURL
	}
	b, err := r.dataset(ctx, "aws-ip-ranges.json", url)
	if err != nil {
		return nil, fmt.Errorf("AWS ranges: %w", err)
	}
	var d struct {
		Prefixes []struct {
			IPPrefix string `json:"ip_prefix"`
			Region   string `json:"region"`
			Service  string `json:"service"`
		} `json:"prefixes"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	svc := map[string]bool{}
	for _, s := range services {
		svc[s] = true
	}
	var out []netip.Prefix
	for _, p := range d.Prefixes {
		if !regions[p.Region] || (len(svc) > 0 && !svc[p.Service]) {
			continue
		}
		if pfx, err := netip.ParsePrefix(p.IPPrefix); err == nil && pfx.Addr().Is4() {
			out = append(out, pfx.Masked())
		}
	}
	return out, nil
}

func (r *Resolver) gcpPrefixes(ctx context.Context, regions map[string]bool) ([]netip.Prefix, error) {
	url := r.GCPRangesURL
	if url == "" {
		url = gcpRangesURL
	}
	b, err := r.dataset(ctx, "gcp-ip-ranges.json", url)
	if err != nil {
		return nil, fmt.Errorf("Google Cloud ranges: %w", err)
	}
	var d struct {
		Prefixes []struct {
			IPv4Prefix string `json:"ipv4Prefix"`
			Scope      string `json:"scope"`
		} `json:"prefixes"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	var out []netip.Prefix
	for _, p := range d.Prefixes {
		if p.IPv4Prefix == "" || !regions[p.Scope] {
			continue
		}
		if pfx, err := netip.ParsePrefix(p.IPv4Prefix); err == nil && pfx.Addr().Is4() {
			out = append(out, pfx.Masked())
		}
	}
	return out, nil
}

// dataset returns a published range file, cached on disk for a day and
// falling back to a stale cache when offline.
func (r *Resolver) dataset(ctx context.Context, name, url string) ([]byte, error) {
	cache := filepath.Join(r.CacheDir, name)
	if fi, err := os.Stat(cache); err == nil && time.Since(fi.ModTime()) < 24*time.Hour {
		if b, err := os.ReadFile(cache); err == nil && json.Valid(b) {
			return b, nil
		}
	}
	hc := r.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	fetchErr := func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := hc.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("download %s: %s", url, resp.Status)
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		if err != nil {
			return err
		}
		if !json.Valid(b) {
			return fmt.Errorf("download %s: invalid JSON", url)
		}
		_ = os.MkdirAll(r.CacheDir, 0o700)
		return os.WriteFile(cache, b, 0o600)
	}()
	if b, err := os.ReadFile(cache); err == nil && json.Valid(b) {
		return b, nil
	}
	if fetchErr != nil {
		return nil, fetchErr
	}
	return nil, errors.New("ranges unavailable")
}
