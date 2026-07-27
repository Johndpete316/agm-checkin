// Command cf-sync mirrors the ip_blocklists table into the exclude list of a
// Cloudflare Access policy, so blocked IPs are turned away at Cloudflare's edge
// instead of after they have already traversed the tunnel and reached a pod.
//
// The target policy is a bypass policy of the form "allow these geos EXCEPT
// these IPs": its include list holds the permitted countries and its exclude
// list holds the blocklist. This job owns exclude and only exclude — include is
// read and written back untouched.
//
// It runs once and exits; scheduling is owned by the k8s CronJob. A non-zero
// exit marks the Job failed and leaves the logs available via kubectl.
package main

import (
	"context"
	"log"
	"net/netip"
	"os"
	"sort"
	"strconv"
	"time"

	"johndpete316/agm-checkin-api/internal/cfaccess"
	"johndpete316/agm-checkin-api/internal/db"
)

// defaultMaxIPs guards against shipping an absurdly large policy if the
// blocklist ever runs away. Overridable with CF_MAX_IPS.
const defaultMaxIPs = 1000

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	dsn := mustEnv("DATABASE_URL")
	accountID := mustEnv("CF_ACCOUNT_ID")
	token := mustEnv("CF_API_TOKEN")
	policyID := mustEnv("CF_POLICY_ID")
	maxIPs := envInt("CF_MAX_IPS", defaultMaxIPs)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	database := db.Connect(dsn)

	var rows []string
	if res := database.Model(&db.IPBlocklist{}).Pluck("ip_address", &rows); res.Error != nil {
		log.Fatalf("reading ip_blocklists: %v", res.Error)
	}

	desired := normalizeAll(rows)

	if len(desired) > maxIPs {
		log.Fatalf("refusing to sync %d IPs, above the CF_MAX_IPS limit of %d", len(desired), maxIPs)
	}

	client := cfaccess.New(accountID, token)

	policy, err := client.GetPolicy(ctx, policyID)
	if err != nil {
		log.Fatalf("fetching policy %s: %v", policyID, err)
	}

	added, removed, kept := applyBlocklist(policy, desired)
	if len(added) == 0 && len(removed) == 0 {
		log.Printf("policy %q already excludes %d IPs; no change", policy.Name, len(desired))
		return
	}

	if err := client.UpdatePolicy(ctx, policyID, policy); err != nil {
		log.Fatalf("updating policy %s: %v", policyID, err)
	}

	log.Printf("synced policy %q: +%d -%d, now excluding %d IPs (%d non-IP exclude rules preserved)",
		policy.Name, len(added), len(removed), len(desired), kept)
}

// applyBlocklist rewrites the policy's exclude list in place so it holds
// exactly the desired CIDRs, and reports what changed plus how many non-IP
// exclude rules were carried through.
//
// The blocklist belongs in exclude, not include. The target policy is a bypass
// policy of the form "Allow FROM US-CA-MX EXCEPT blocked_ips": include holds
// the permitted geos, and excluding an IP is what denies it the bypass. Writing
// the IPs into include would instead grant blocked addresses the bypass — the
// exact opposite of blocking them — and destroy the geo rules on the way. So
// include is never touched here.
func applyBlocklist(policy *cfaccess.Policy, desired []string) (added, removed []string, kept int) {
	current, others := splitRules(policy.Exclude)

	added, removed = diff(current, desired)

	// Non-IP rules first, exactly as Cloudflare returned them, then our IP
	// rules sorted for a stable diff in the dashboard. An empty result is
	// legitimate — it means nothing is blocked — and is accepted because
	// include still carries the geo rules.
	exclude := make([]cfaccess.Rule, 0, len(others)+len(desired))
	exclude = append(exclude, others...)
	for _, ip := range desired {
		exclude = append(exclude, cfaccess.IPRuleFor(ip))
	}
	policy.Exclude = exclude

	return added, removed, len(others)
}

// normalizeAll converts stored addresses to sorted, deduplicated CIDR blocks.
// Unparseable rows are logged and skipped — one bad row should not stop the
// rest of the blocklist from reaching the edge.
func normalizeAll(rows []string) []string {
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		cidr, err := normalizeIP(row)
		if err != nil {
			log.Printf("skipping unparseable ip_blocklists entry %q: %v", row, err)
			continue
		}
		seen[cidr] = struct{}{}
	}

	out := make([]string, 0, len(seen))
	for cidr := range seen {
		out = append(out, cidr)
	}
	sort.Strings(out)
	return out
}

// normalizeIP widens a bare address to a single-host CIDR block, which is what
// the Access "ip" selector expects. Values already in CIDR form pass through
// in their canonical spelling.
func normalizeIP(s string) (string, error) {
	if prefix, err := netip.ParsePrefix(s); err == nil {
		return prefix.Masked().String(), nil
	}

	addr, err := netip.ParseAddr(s)
	if err != nil {
		return "", err
	}
	// Unmap so an IPv4-in-IPv6 address (::ffff:1.2.3.4) becomes a /32, not a /128.
	addr = addr.Unmap()

	return netip.PrefixFrom(addr, addr.BitLen()).String(), nil
}

// splitRules separates the IP rules we manage from every other selector,
// which is returned untouched so it can be written back verbatim.
func splitRules(rules []cfaccess.Rule) (ips []string, others []cfaccess.Rule) {
	for _, r := range rules {
		if ip, ok := r.IP(); ok {
			ips = append(ips, ip)
			continue
		}
		others = append(others, r)
	}
	sort.Strings(ips)
	return ips, others
}

func diff(current, desired []string) (added, removed []string) {
	cur := make(map[string]struct{}, len(current))
	for _, c := range current {
		cur[c] = struct{}{}
	}
	want := make(map[string]struct{}, len(desired))
	for _, d := range desired {
		want[d] = struct{}{}
	}

	for _, d := range desired {
		if _, ok := cur[d]; !ok {
			added = append(added, d)
		}
	}
	for _, c := range current {
		if _, ok := want[c]; !ok {
			removed = append(removed, c)
		}
	}
	return added, removed
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s environment variable is required", key)
	}
	return v
}

func envInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		log.Fatalf("%s must be a positive integer, got %q", key, raw)
	}
	return n
}
