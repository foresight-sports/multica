package handler

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJiraMCPAuthPageEscapesContentAndAllowsOnlyItsStyle(t *testing.T) {
	h := *testHandler
	h.cfg.PublicURL = "https://multica.example"
	w := httptest.NewRecorder()
	h.jiraAuthPage(w, 200, jiraAuthPageData{Title: "Connect Jira", Email: "<script>alert(1)</script>", State: "test-state", Nonce: "test-csrf"})
	body := w.Body.String()
	if strings.Contains(body, "<script>") || !strings.Contains(body, "&lt;script&gt;") || !strings.Contains(body, `name="csrf" value="test-csrf"`) {
		t.Fatal("account content must be escaped and CSRF form preserved")
	}
	csp := w.Header().Get("Content-Security-Policy")
	var formAction string
	for _, directive := range strings.Split(csp, ";") {
		if strings.HasPrefix(strings.TrimSpace(directive), "form-action ") {
			formAction = strings.TrimSpace(directive)
		}
	}
	if formAction != "form-action 'self' https://mcp.atlassian.com https://auth.atlassian.com https://id.atlassian.com" {
		t.Fatal("confirmation must permit the Atlassian OAuth redirect chain without permitting unrelated destinations")
	}
	prefix := "style-src 'nonce-"
	_, tail, ok := strings.Cut(csp, prefix)
	nonce, _, _ := strings.Cut(tail, "'")
	if !ok || nonce == "" || !strings.Contains(body, `style nonce="`+nonce+`"`) || !strings.Contains(csp, "form-action 'self'") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("page must authorize only its own style and remain uncacheable")
	}
	w = httptest.NewRecorder()
	h.jiraAuthError(w, 400, "Sign-in expired")
	if w.Code != 400 || strings.Contains(w.Body.String(), "<form") || !strings.Contains(w.Body.String(), "Return to Multica") {
		t.Fatal("expired page must preserve status and offer recovery without a consent form")
	}
}
