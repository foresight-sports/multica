package agent

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/pkg/quota"
)

type claudeQuotaTransport func(*http.Request) (*http.Response, error)

func (f claudeQuotaTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestClaudeAccountUsage(t *testing.T) {
	now := time.Now().UTC()
	base := quota.Report{Provider: "claude", Source: "claude_usage", Status: "unknown", ObservedAt: now, Windows: []quota.Window{}}
	client := &http.Client{Transport: claudeQuotaTransport(func(req *http.Request) (*http.Response, error) {
		if req.Method != "GET" || req.URL.String() != "https://api.anthropic.com/api/oauth/usage" || req.Header.Get("Authorization") != "Bearer fake-secret" || req.Header.Get("anthropic-beta") != "oauth-2025-04-20" {
			t.Fatalf("unexpected account request")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"five_hour":{"utilization":42,"resets_at":"2099-01-01T00:00:00Z"},"seven_day":{"utilization":100},"seven_day_opus":null,"seven_day_sonnet":{"utilization":0},"extra_usage":{"is_enabled":false},"future_field":"ignored", "future_array":[1,2]}`))}, nil
	})}
	r, err := readClaudeUsage(context.Background(), []byte(`{"claudeAiOauth":{"accessToken":"fake-secret","scopes":["user:profile"]}}`), client, base)
	if err != nil || r.Status != "reported" || len(r.Windows) != 3 || *r.Windows[0].UsedPercent != 42 || *r.Windows[2].UsedPercent != 0 || !r.Blocks("any-model", now) || r.AccountKey != "" {
		t.Fatalf("invalid report: %+v %v", r, err)
	}
	for _, input := range []string{`{}`, `{"claudeAiOauth":{"accessToken":"fake-secret","scopes":[]}}`, `{"claudeAiOauth":{"accessToken":"fake-secret","scopes":["user:profile"],"expiresAt":1}}`} {
		_, err := readClaudeUsage(context.Background(), []byte(input), &http.Client{Transport: claudeQuotaTransport(func(*http.Request) (*http.Response, error) {
			t.Fatal("invalid login must not issue request")
			return nil, nil
		})}, base)
		if err == nil || strings.Contains(err.Error(), "fake-secret") {
			t.Fatal("invalid credentials accepted or leaked")
		}
	}
	client.Transport = claudeQuotaTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader("private response"))}, nil
	})
	r, err = readClaudeUsage(context.Background(), []byte(`{"claudeAiOauth":{"accessToken":"fake-secret","scopes":["user:profile"]}}`), client, base)
	if err == nil || err.Error() != "Claude usage request returned HTTP 429" || r.Status != "unknown" {
		t.Fatalf("HTTP failure: %+v %v", r, err)
	}
}

func TestClaudeUsageMissingValues(t *testing.T) {
	base := quota.Report{Provider: "claude", Source: "claude_usage", Status: "unknown", ObservedAt: time.Now().UTC(), Windows: []quota.Window{}}
	r, err := parseClaudeUsage([]byte(`{"five_hour":{"utilization":null,"resets_at":null},"seven_day":null}`), base)
	if err != nil || len(r.Windows) != 1 || r.Windows[0].UsedPercent != nil || r.Blocks("m", time.Now()) {
		t.Fatalf("missing data became usage: %+v %v", r, err)
	}
	for _, raw := range []string{`{"five_hour":{"utilization":-1}}`, `{"five_hour":{"resets_at":"bad"}}`, `{"five_hour":{"utilization":"100"}}`} {
		if _, err := parseClaudeUsage([]byte(raw), base); err == nil {
			t.Fatalf("accepted malformed response %s", raw)
		}
	}
	r, err = parseClaudeUsage([]byte(`{"seven_day_sonnet":{"utilization":100}}`), base)
	if err != nil || !r.Blocks("claude-sonnet-4-6", base.ObservedAt) || r.Blocks("claude-opus-4-6", base.ObservedAt) {
		t.Fatal("model-specific capacity must only block its family")
	}
}
