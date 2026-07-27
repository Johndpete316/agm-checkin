package cfaccess

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A non-IP selector must survive a decode/encode cycle byte-for-byte. If it
// does not, syncing would silently delete rules we do not model.
func TestRuleRoundTripsUnknownSelectors(t *testing.T) {
	cases := []string{
		`{"email":{"email":"staff@example.com"}}`,
		`{"everyone":{}}`,
		`{"geo":{"country_code":"US"}}`,
		`{"group":{"id":"abc123"}}`,
	}

	for _, in := range cases {
		var r Rule
		if err := json.Unmarshal([]byte(in), &r); err != nil {
			t.Fatalf("unmarshal %s: %v", in, err)
		}
		if _, ok := r.IP(); ok {
			t.Errorf("%s was misidentified as an IP rule", in)
		}

		out, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("marshal %s: %v", in, err)
		}
		if string(out) != in {
			t.Errorf("round trip changed rule:\n got %s\nwant %s", out, in)
		}
	}
}

func TestRuleIdentifiesIPSelector(t *testing.T) {
	var r Rule
	if err := json.Unmarshal([]byte(`{"ip":{"ip":"203.0.113.7/32"}}`), &r); err != nil {
		t.Fatal(err)
	}
	ip, ok := r.IP()
	if !ok {
		t.Fatal("expected an IP rule")
	}
	if ip != "203.0.113.7/32" {
		t.Errorf("got %q, want 203.0.113.7/32", ip)
	}
}

func TestIPRuleForMarshals(t *testing.T) {
	out, err := json.Marshal(IPRuleFor("198.51.100.4/32"))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"ip":{"ip":"198.51.100.4/32"}}`
	if string(out) != want {
		t.Errorf("got %s, want %s", out, want)
	}
}

// Cloudflare can return success:false on an HTTP 200. Treating that as a
// successful read would make the sync think the policy had no rules and
// replace the whole include list.
func TestSuccessFalseOnHTTP200IsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":10000,"message":"Authentication error"}],"result":null}`))
	}))
	defer srv.Close()

	c := New("acct", "tok")
	c.http = srv.Client()

	_, err := c.do(context.Background(), http.MethodGet, srv.URL, nil)
	if err == nil {
		t.Fatal("expected an error for success:false")
	}
	if !strings.Contains(err.Error(), "Authentication error") {
		t.Errorf("error should surface the Cloudflare message, got: %v", err)
	}
}

func TestNonJSONBodyIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>502 Bad Gateway</html>"))
	}))
	defer srv.Close()

	c := New("acct", "tok")
	c.http = srv.Client()

	if _, err := c.do(context.Background(), http.MethodGet, srv.URL, nil); err == nil {
		t.Fatal("expected an error for a non-JSON body")
	}
}

func TestGetPolicySendsBearerToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"success":true,"result":{"name":"Blocked IPs","decision":"block","include":[{"ip":{"ip":"203.0.113.1/32"}}]}}`))
	}))
	defer srv.Close()

	c := New("acct", "secret-token")
	c.http = srv.Client()

	body, err := c.do(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("got auth header %q", gotAuth)
	}

	var p Policy
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "Blocked IPs" || p.Decision != "block" || len(p.Include) != 1 {
		t.Errorf("policy decoded incorrectly: %+v", p)
	}
}

// Guards against a bug that would blank the policy name or reset its decision.
func TestUpdatePolicyRejectsUnsafePayloads(t *testing.T) {
	c := New("acct", "tok")

	cases := map[string]*Policy{
		"missing name":     {Decision: "block", Include: []Rule{IPRuleFor("203.0.113.1/32")}},
		"missing decision": {Name: "Blocked IPs", Include: []Rule{IPRuleFor("203.0.113.1/32")}},
		"empty include":    {Name: "Blocked IPs", Decision: "block"},
	}

	for name, p := range cases {
		if err := c.UpdatePolicy(context.Background(), "pid", p); err == nil {
			t.Errorf("%s: expected a refusal", name)
		}
	}
}
