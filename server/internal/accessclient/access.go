// Package accessclient authenticates outbound Multica clients to Cloudflare Access.
package accessclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type credentials struct {
	Origin       string `json:"origin"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// Apply loads credentials on each request so rotation does not require a restart.
// Missing default configuration leaves ordinary Multica installations unchanged.
func Apply(target string, headers http.Header) error {
	headers.Del("CF-Access-Client-Id")
	headers.Del("CF-Access-Client-Secret")
	path := os.Getenv("MULTICA_CLOUDFLARE_ACCESS_FILE")
	explicit := path != ""
	if !explicit {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot locate Cloudflare Access configuration")
		}
		path = filepath.Join(home, ".multica", "cloudflare-access.json")
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) && !explicit {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read Cloudflare Access credential file")
	}
	var c credentials
	if json.Unmarshal(data, &c) != nil {
		return fmt.Errorf("invalid Cloudflare Access credential file")
	}
	origin, err := url.Parse(c.Origin)
	if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil || (origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" || origin.Fragment != "" {
		return fmt.Errorf("Cloudflare Access origin must be an HTTPS origin without a path")
	}
	if strings.TrimSpace(c.ClientID) == "" || strings.TrimSpace(c.ClientSecret) == "" || strings.ContainsAny(c.ClientID+c.ClientSecret, "\r\n") {
		return fmt.Errorf("Cloudflare Access credentials are incomplete or invalid")
	}
	u, err := url.Parse(target)
	if err != nil {
		return fmt.Errorf("invalid Cloudflare Access request URL")
	}
	if (u.Scheme == "https" || u.Scheme == "wss") && strings.EqualFold(u.Host, origin.Host) && u.User == nil {
		headers.Set("CF-Access-Client-Id", c.ClientID)
		headers.Set("CF-Access-Client-Secret", c.ClientSecret)
	}
	return nil
}

type transport struct{ base http.RoundTripper }

// Transport scopes credentials for every hop, including redirects and downloads.
func Transport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &transport{base: base}
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	if err := Apply(copy.URL.String(), copy.Header); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(copy)
}

func (t *transport) CloseIdleConnections() {
	if c, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}
