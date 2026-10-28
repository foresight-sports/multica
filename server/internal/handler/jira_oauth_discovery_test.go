package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestJiraOAuthUsesResourceAuthorizationServer(t *testing.T) {
	var origin string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource/v2/mcp":
			json.NewEncoder(w).Encode(map[string]any{"resource": jiraMCPUpstream + "/v2/mcp", "authorization_servers": []string{origin + "/tenant"}})
		case "/.well-known/oauth-authorization-server/tenant":
			json.NewEncoder(w).Encode(jiraOAuthMetadata{Issuer: origin + "/tenant", Authorization: origin + "/authorize", Token: origin + "/oauth/token", Registration: origin + "/dcr/register"})
		case "/oauth/token":
			r.ParseForm()
			if r.Form.Get("resource") != jiraMCPUpstream+"/v2/mcp" {
				t.Error("wrong resource")
			}
			json.NewEncoder(w).Encode(map[string]any{"access_token": "new-token", "expires_in": 3600, "token_type": "Bearer"})
		default:
			t.Errorf("unexpected legacy endpoint: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	origin = server.URL
	j := &JiraMCP{BaseURL: origin, Client: server.Client()}
	metadata, err := j.discoverOAuth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := j.exchange(context.Background(), jiraOAuthCredentials{ClientID: "client", TokenEndpoint: metadata.Token}, url.Values{"grant_type": {"authorization_code"}, "code": {"test"}})
	if err != nil || c.AccessToken != "new-token" {
		t.Fatalf("exchange failed: %v", err)
	}
	for _, endpoint := range []string{"https://attacker.example/token", "https://auth.atlassian.com.attacker.example/token", "https://user:password@auth.atlassian.com/token", "http://auth.atlassian.com/token"} {
		if j.trustedOAuthURL(endpoint) {
			t.Errorf("trusted %s", endpoint)
		}
	}
}
