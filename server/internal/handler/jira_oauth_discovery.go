package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type jiraOAuthMetadata struct {
	Issuer        string `json:"issuer"`
	Authorization string `json:"authorization_endpoint"`
	Token         string `json:"token_endpoint"`
	Registration  string `json:"registration_endpoint"`
}

// Discovery is restricted to Atlassian and the configured MCP origin. Never
// follow redirects or send tokens to an arbitrary metadata endpoint.
func (j *JiraMCP) trustedOAuthURL(raw string) bool {
	u, err := url.Parse(raw)
	base, baseErr := url.Parse(j.BaseURL)
	if err != nil || baseErr != nil || u.User != nil || u.Fragment != "" {
		return false
	}
	return (u.Scheme == base.Scheme && u.Host == base.Host) || (u.Scheme == "https" && u.Host == "auth.atlassian.com")
}

func (j *JiraMCP) oauthMetadata(ctx context.Context, endpoint string, out any) error {
	if !j.trustedOAuthURL(endpoint) {
		return errors.New("untrusted OAuth metadata")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := j.Client.Do(req)
	if err != nil {
		return errors.New("OAuth discovery unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("OAuth discovery unavailable")
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}

func (j *JiraMCP) discoverOAuth(ctx context.Context) (jiraOAuthMetadata, error) {
	var metadata jiraOAuthMetadata
	var resource struct {
		Resource string   `json:"resource"`
		Servers  []string `json:"authorization_servers"`
	}
	if err := j.oauthMetadata(ctx, j.BaseURL+"/.well-known/oauth-protected-resource/v2/mcp", &resource); err != nil {
		return metadata, err
	}
	if resource.Resource != jiraMCPUpstream+"/v2/mcp" || len(resource.Servers) != 1 || !j.trustedOAuthURL(resource.Servers[0]) {
		return metadata, errors.New("invalid OAuth resource metadata")
	}
	issuer, _ := url.Parse(resource.Servers[0])
	if issuer.RawQuery != "" {
		return metadata, errors.New("invalid OAuth issuer")
	}
	endpoint := issuer.Scheme + "://" + issuer.Host + "/.well-known/oauth-authorization-server" + strings.TrimRight(issuer.Path, "/")
	if err := j.oauthMetadata(ctx, endpoint, &metadata); err != nil {
		return metadata, err
	}
	if metadata.Issuer != resource.Servers[0] || !j.trustedOAuthURL(metadata.Authorization) || !j.trustedOAuthURL(metadata.Token) || !j.trustedOAuthURL(metadata.Registration) {
		return metadata, errors.New("invalid OAuth server metadata")
	}
	return metadata, nil
}
