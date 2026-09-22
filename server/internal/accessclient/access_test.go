package accessclient

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func config(t *testing.T, body string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "access.json")
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MULTICA_CLOUDFLARE_ACCESS_FILE", p)
}

func TestCredentialScope(t *testing.T) {
	config(t, `{"origin":"https://multica.example","client_id":"id","client_secret":"secret"}`)
	for _, tc := range []struct {
		url  string
		want bool
	}{
		{"https://multica.example/api/tokens", true},
		{"wss://multica.example/api/daemon/ws", true},
		{"http://multica.example/api/tokens", false},
		{"https://multica.example:444/api/tokens", false},
		{"https://evil.multica.example/api/tokens", false},
		{"https://storage.example/file", false},
	} {
		h := http.Header{}
		h.Set("CF-Access-Client-Secret", "copied-by-redirect")
		h.Set("Authorization", "Bearer multica-token")
		if err := Apply(tc.url, h); err != nil {
			t.Fatal(err)
		}
		if got := h.Get("CF-Access-Client-Secret"); (got == "secret") != tc.want || (!tc.want && got != "") {
			t.Errorf("%s: secret scope mismatch", tc.url)
		}
		if h.Get("Authorization") != "Bearer multica-token" {
			t.Fatal("Multica authentication changed")
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRedirectDoesNotLeak(t *testing.T) {
	config(t, `{"origin":"https://multica.example","client_id":"id","client_secret":"secret"}`)
	calls := 0
	c := &http.Client{Transport: Transport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("ok")), Request: r}
		if calls == 1 {
			if r.Header.Get("CF-Access-Client-Secret") != "secret" {
				t.Fatal("missing origin credential")
			}
			resp.StatusCode = 302
			resp.Header.Set("Location", "https://login.example/login")
		} else if r.Header.Get("CF-Access-Client-Secret") != "" {
			t.Fatal("credential leaked on redirect")
		}
		return resp, nil
	}))}
	r, err := c.Get("https://multica.example/api/tokens")
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if calls != 2 {
		t.Fatalf("expected redirect, got %d calls", calls)
	}
}

func TestInvalidConfigFailsWithoutSecrets(t *testing.T) {
	for _, body := range []string{`bad secret`, `{"origin":"http://multica.example","client_id":"id","client_secret":"secret"}`, `{"origin":"https://multica.example","client_id":"id"}`} {
		config(t, body)
		err := Apply("https://multica.example/api/me", http.Header{})
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("expected sanitized configuration error, got %v", err)
		}
	}
}
