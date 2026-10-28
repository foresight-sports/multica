package handler

import "testing"

func TestJiraReturnPath(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"https://multica.example/fs-app/issues/APP-9?tab=activity", "/fs-app/issues/APP-9?tab=activity"},
		{"/fs-app/inbox?issue=abc", "/fs-app/inbox?issue=abc"},
		{"/settings/account#jira", "/settings/account#jira"},
		{"https://evil.example/path", "/"}, {"//evil.example/path", "/"}, {"/%2fhost/path", "/"},
		{"/\\evil.example", "/"}, {"/api/integrations/jira/callback?code=secret", "/"}, {"", "/"},
	} {
		if got := jiraReturnPath("https://multica.example", tc.input); got != tc.want {
			t.Errorf("input %q: got %q want %q", tc.input, got, tc.want)
		}
	}
}
