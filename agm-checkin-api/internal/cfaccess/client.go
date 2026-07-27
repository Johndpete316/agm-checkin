// Package cfaccess is a minimal client for the Cloudflare Zero Trust Access
// policy API — just enough to read a policy and replace its include rules.
// Stdlib only; the full cloudflare-go SDK is far more than this needs.
package cfaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const baseURL = "https://api.cloudflare.com/client/v4"

type Client struct {
	accountID string
	token     string
	http      *http.Client
}

func New(accountID, token string) *Client {
	return &Client{
		accountID: accountID,
		token:     token,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
}

// IPRule is the value side of an "ip" selector. The address must be a CIDR
// block — see NormalizeIP in the cf-sync binary for how bare addresses are
// widened to /32 and /128.
type IPRule struct {
	IP string `json:"ip"`
}

// Rule is one entry in a policy's include/exclude/require array.
//
// Cloudflare supports many selectors (email, group, geo, everyone, ...) and we
// only care about "ip". Decoding into a struct with just an IP field would
// re-encode every other selector as an empty object and quietly destroy it, so
// Rule keeps the original JSON and replays it verbatim on write. Only rules we
// construct ourselves via IPRuleFor are emitted from the typed field.
type Rule struct {
	raw json.RawMessage
	ip  *IPRule
}

// IPRuleFor builds an include rule for a single CIDR block.
func IPRuleFor(cidr string) Rule {
	return Rule{ip: &IPRule{IP: cidr}}
}

// IP returns the rule's CIDR block and whether this rule is an "ip" selector.
func (r Rule) IP() (string, bool) {
	if r.ip == nil {
		return "", false
	}
	return r.ip.IP, true
}

func (r *Rule) UnmarshalJSON(b []byte) error {
	r.raw = append(r.raw[:0], b...)

	// Presence of a non-null "ip" key is what makes this an IP rule; every
	// other selector leaves r.ip nil and survives via raw.
	var probe struct {
		IP *IPRule `json:"ip"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		// An unrecognised shape is still worth preserving verbatim.
		return nil
	}
	r.ip = probe.IP
	return nil
}

func (r Rule) MarshalJSON() ([]byte, error) {
	if r.raw != nil {
		return r.raw, nil
	}
	return json.Marshal(struct {
		IP *IPRule `json:"ip,omitempty"`
	}{IP: r.ip})
}

// Policy is the subset of an Access policy this tool round-trips.
//
// None of the rule arrays carry omitempty on purpose: emptying Exclude is a
// meaningful state change (the last blocked IP was lifted), and with omitempty
// the field would be dropped from the payload and Cloudflare would keep the
// stale list forever.
type Policy struct {
	Name     string `json:"name"`
	Decision string `json:"decision"`
	Include  []Rule `json:"include"`
	Exclude  []Rule `json:"exclude"`
	Require  []Rule `json:"require"`
}

// apiResponse is Cloudflare's standard envelope. Note that success can be
// false on an HTTP 200, so every call checks this field rather than relying on
// the status code alone.
type apiResponse struct {
	Success bool            `json:"success"`
	Errors  []apiError      `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e apiError) String() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

func (c *Client) policyURL(policyID string) string {
	return fmt.Sprintf("%s/accounts/%s/access/policies/%s", baseURL, c.accountID, policyID)
}

func (c *Client) GetPolicy(ctx context.Context, policyID string) (*Policy, error) {
	body, err := c.do(ctx, http.MethodGet, c.policyURL(policyID), nil)
	if err != nil {
		return nil, err
	}

	var p Policy
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("decoding policy: %w", err)
	}
	return &p, nil
}

// UpdatePolicy replaces the policy in place. Cloudflare requires name and
// decision on every write, so callers must carry them over from GetPolicy —
// omitting them would blank the name and reset the decision.
func (c *Client) UpdatePolicy(ctx context.Context, policyID string, p *Policy) error {
	if p.Name == "" || p.Decision == "" {
		return fmt.Errorf("refusing to update policy %s with empty name or decision", policyID)
	}
	// Include is never managed by this tool — it carries the policy's geo
	// rules. An empty one means we decoded the policy wrongly, and writing it
	// would strip those rules, so refuse. Exclude may legitimately be empty.
	if len(p.Include) == 0 {
		return fmt.Errorf("refusing to update policy %s with an empty include list", policyID)
	}

	payload, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("encoding policy: %w", err)
	}

	_, err = c.do(ctx, http.MethodPut, c.policyURL(policyID), payload)
	return err
}

// do issues the request and unwraps Cloudflare's response envelope, returning
// the raw `result` field.
func (c *Client) do(ctx context.Context, method, url string, body []byte) (json.RawMessage, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var env apiResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		// A non-JSON body means we never reached the API proper (proxy error,
		// HTML error page). Surface the status and a snippet.
		return nil, fmt.Errorf("%s returned HTTP %d with unparseable body: %s",
			method, resp.StatusCode, truncate(string(raw), 200))
	}

	if !env.Success {
		return nil, fmt.Errorf("cloudflare API error (HTTP %d): %s",
			resp.StatusCode, joinErrors(env.Errors))
	}

	return env.Result, nil
}

func joinErrors(errs []apiError) string {
	if len(errs) == 0 {
		return "no error detail returned"
	}
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		parts = append(parts, e.String())
	}
	return strings.Join(parts, "; ")
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
