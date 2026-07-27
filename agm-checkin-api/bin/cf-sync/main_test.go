package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"johndpete316/agm-checkin-api/internal/cfaccess"
)

func TestNormalizeIP(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "203.0.113.7", want: "203.0.113.7/32"},
		{in: "203.0.113.7/32", want: "203.0.113.7/32"},
		{in: "10.0.0.0/8", want: "10.0.0.0/8"},
		// Host bits set: Cloudflare wants the network address.
		{in: "10.1.2.3/8", want: "10.0.0.0/8"},
		{in: "2001:db8::1", want: "2001:db8::1/128"},
		{in: "2001:db8::/64", want: "2001:db8::/64"},
		// An IPv4-mapped IPv6 address should become a /32, not a /128.
		{in: "::ffff:203.0.113.7", want: "203.0.113.7/32"},
		{in: "", wantErr: true},
		{in: "not-an-ip", wantErr: true},
		{in: "203.0.113.999", wantErr: true},
		{in: "203.0.113.7/33", wantErr: true},
	}

	for _, tc := range cases {
		got, err := normalizeIP(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("normalizeIP(%q) = %q, want an error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeIP(%q) returned %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("normalizeIP(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeAllSortsDedupesAndSkipsGarbage(t *testing.T) {
	got := normalizeAll([]string{
		"203.0.113.7",
		"198.51.100.1",
		"203.0.113.7/32", // duplicate of the first once normalized
		"garbage",
		"198.51.100.1",
	})

	want := []string{"198.51.100.1/32", "203.0.113.7/32"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestNormalizeAllEmpty(t *testing.T) {
	if got := normalizeAll(nil); len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
	if got := normalizeAll([]string{"nonsense"}); len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestSplitRulesPreservesNonIPRules(t *testing.T) {
	raw := `[
		{"ip":{"ip":"203.0.113.7/32"}},
		{"email":{"email":"staff@example.com"}},
		{"ip":{"ip":"198.51.100.1/32"}},
		{"everyone":{}}
	]`

	var rules []cfaccess.Rule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		t.Fatal(err)
	}

	ips, others := splitRules(rules)

	wantIPs := []string{"198.51.100.1/32", "203.0.113.7/32"}
	if !reflect.DeepEqual(ips, wantIPs) {
		t.Errorf("ips = %v, want %v", ips, wantIPs)
	}
	if len(others) != 2 {
		t.Fatalf("expected 2 non-IP rules, got %d", len(others))
	}

	// The preserved rules must re-encode exactly as they arrived.
	out, err := json.Marshal(others)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"email":{"email":"staff@example.com"}},{"everyone":{}}]`
	if string(out) != want {
		t.Errorf("preserved rules changed:\n got %s\nwant %s", out, want)
	}
}

// realPolicyJSON is the live policy as the Cloudflare API actually returns it.
// Note decision "bypass" and geo rules in include — the blocklist belongs in
// exclude.
const realPolicyJSON = `{
	"name": "Allow FROM US-CA-MX EXCEPT blocked_ips",
	"decision": "bypass",
	"include": [
		{"geo":{"country_code":"US"}},
		{"geo":{"country_code":"MX"}},
		{"geo":{"country_code":"CA"}}
	],
	"exclude": [],
	"require": []
}`

func mustRealPolicy(t *testing.T) *cfaccess.Policy {
	t.Helper()
	var p cfaccess.Policy
	if err := json.Unmarshal([]byte(realPolicyJSON), &p); err != nil {
		t.Fatal(err)
	}
	return &p
}

// The bug this guards against would have written the blocklist into include,
// granting blocked IPs the bypass and wiping the geo restriction.
func TestApplyBlocklistWritesToExcludeAndPreservesGeoInclude(t *testing.T) {
	p := mustRealPolicy(t)

	added, removed, kept := applyBlocklist(p, []string{"198.51.100.1/32", "203.0.113.7/32"})

	if len(added) != 2 || len(removed) != 0 || kept != 0 {
		t.Errorf("added=%v removed=%v kept=%d", added, removed, kept)
	}

	// Include must come back byte-identical: three geo rules, no IPs.
	gotInclude, err := json.Marshal(p.Include)
	if err != nil {
		t.Fatal(err)
	}
	wantInclude := `[{"geo":{"country_code":"US"}},{"geo":{"country_code":"MX"}},{"geo":{"country_code":"CA"}}]`
	if string(gotInclude) != wantInclude {
		t.Errorf("include was modified:\n got %s\nwant %s", gotInclude, wantInclude)
	}

	gotExclude, err := json.Marshal(p.Exclude)
	if err != nil {
		t.Fatal(err)
	}
	wantExclude := `[{"ip":{"ip":"198.51.100.1/32"}},{"ip":{"ip":"203.0.113.7/32"}}]`
	if string(gotExclude) != wantExclude {
		t.Errorf("exclude wrong:\n got %s\nwant %s", gotExclude, wantExclude)
	}

	if p.Name != "Allow FROM US-CA-MX EXCEPT blocked_ips" || p.Decision != "bypass" {
		t.Errorf("name/decision were altered: %q / %q", p.Name, p.Decision)
	}
}

// Lifting the last block must serialise as an explicit empty array. With
// omitempty on Exclude the field would vanish and Cloudflare would keep the
// stale blocklist forever.
func TestApplyBlocklistClearingSerialisesEmptyExclude(t *testing.T) {
	p := mustRealPolicy(t)
	applyBlocklist(p, []string{"203.0.113.7/32"})

	added, removed, _ := applyBlocklist(p, nil)
	if len(added) != 0 || len(removed) != 1 {
		t.Errorf("added=%v removed=%v, want one removal", added, removed)
	}

	out, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"exclude":[]`) {
		t.Errorf("expected an explicit empty exclude array, got: %s", out)
	}
}

func TestApplyBlocklistIsIdempotent(t *testing.T) {
	p := mustRealPolicy(t)
	ips := []string{"198.51.100.1/32", "203.0.113.7/32"}

	applyBlocklist(p, ips)
	added, removed, _ := applyBlocklist(p, ips)

	if len(added) != 0 || len(removed) != 0 {
		t.Errorf("second apply reported changes: added=%v removed=%v", added, removed)
	}
}

// A non-IP exclude rule added by hand in the dashboard must survive a sync.
func TestApplyBlocklistPreservesNonIPExcludeRules(t *testing.T) {
	p := mustRealPolicy(t)
	if err := json.Unmarshal([]byte(`[{"email":{"email":"staff@example.com"}}]`), &p.Exclude); err != nil {
		t.Fatal(err)
	}

	_, _, kept := applyBlocklist(p, []string{"203.0.113.7/32"})
	if kept != 1 {
		t.Fatalf("kept = %d, want 1", kept)
	}

	out, err := json.Marshal(p.Exclude)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"email":{"email":"staff@example.com"}},{"ip":{"ip":"203.0.113.7/32"}}]`
	if string(out) != want {
		t.Errorf("got %s, want %s", out, want)
	}
}

func TestDiff(t *testing.T) {
	cases := []struct {
		name                  string
		current, desired      []string
		wantAdded, wantRemove []string
	}{
		{
			name:    "no change",
			current: []string{"a", "b"},
			desired: []string{"a", "b"},
		},
		{
			name:      "added only",
			current:   []string{"a"},
			desired:   []string{"a", "b"},
			wantAdded: []string{"b"},
		},
		{
			name:       "removed only",
			current:    []string{"a", "b"},
			desired:    []string{"a"},
			wantRemove: []string{"b"},
		},
		{
			name:       "both",
			current:    []string{"a", "b"},
			desired:    []string{"b", "c"},
			wantAdded:  []string{"c"},
			wantRemove: []string{"a"},
		},
		{
			name:      "from empty",
			current:   nil,
			desired:   []string{"a"},
			wantAdded: []string{"a"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			added, removed := diff(tc.current, tc.desired)
			if !equalSlices(added, tc.wantAdded) {
				t.Errorf("added = %v, want %v", added, tc.wantAdded)
			}
			if !equalSlices(removed, tc.wantRemove) {
				t.Errorf("removed = %v, want %v", removed, tc.wantRemove)
			}
		})
	}
}

func equalSlices(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}
