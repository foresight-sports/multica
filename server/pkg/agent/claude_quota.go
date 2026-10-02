package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/multica-ai/multica/server/pkg/quota"
)

// ReadClaudeQuota reads the same account usage endpoint as Claude Code's /usage.
// It never starts an inference session, refreshes tokens, or writes credentials.
func ReadClaudeQuota(ctx context.Context) (quota.Report, error) {
	r := quota.Report{Provider: "claude", Source: "claude_usage", Status: "unknown", ObservedAt: time.Now().UTC(), Windows: []quota.Window{}}
	// Never attribute a saved subscription login to a custom/API-authenticated CLI.
	for _, key := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL", "ANTHROPIC_PROFILE", "CLAUDE_CODE_OAUTH_TOKEN", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY", "CLAUDE_CODE_CUSTOM_OAUTH_URL"} {
		if os.Getenv(key) != "" {
			return r, fmt.Errorf("Claude capacity unavailable with custom authentication")
		}
	}
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return r, fmt.Errorf("Claude login directory unavailable")
		}
		dir = filepath.Join(home, ".claude")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// Global helper/env settings can select a different authentication source.
	// Do not execute helpers or borrow another account's subscription figures.
	if raw, err := os.ReadFile(filepath.Join(dir, "settings.json")); err == nil {
		var settings struct {
			APIKeyHelper string                     `json:"apiKeyHelper"`
			Env          map[string]json.RawMessage `json:"env"`
		}
		if json.Unmarshal(raw, &settings) != nil || settings.APIKeyHelper != "" || len(settings.Env) > 0 {
			return r, fmt.Errorf("Claude capacity unavailable with custom authentication settings")
		}
	} else if !os.IsNotExist(err) {
		return r, fmt.Errorf("Claude authentication settings unavailable")
	}
	var raw []byte
	if runtime.GOOS == "darwin" {
		// Custom config directories use a different keychain namespace. Do not
		// accidentally read the default account for one of those installations.
		if os.Getenv("CLAUDE_CONFIG_DIR") != "" {
			return r, fmt.Errorf("Claude capacity unavailable for custom keychain namespace")
		}
		cmd := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", "Claude Code-credentials", "-w")
		raw, _ = cmd.Output()
	}
	if len(raw) == 0 {
		f, err := os.Open(filepath.Join(dir, ".credentials.json"))
		if err != nil {
			return r, fmt.Errorf("Claude subscription login unavailable; sign in with Claude Code on this computer")
		}
		defer f.Close()
		raw, _ = io.ReadAll(io.LimitReader(f, 65537))
	}
	return readClaudeUsage(ctx, raw, &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, r)
}

func readClaudeUsage(ctx context.Context, credentials []byte, client *http.Client, r quota.Report) (quota.Report, error) {
	var login struct {
		OAuth *struct {
			AccessToken string   `json:"accessToken"`
			ExpiresAt   int64    `json:"expiresAt"`
			Scopes      []string `json:"scopes"`
		} `json:"claudeAiOauth"`
	}
	if len(credentials) > 65536 || json.Unmarshal(credentials, &login) != nil || login.OAuth == nil || login.OAuth.AccessToken == "" {
		return r, fmt.Errorf("Claude subscription login unavailable")
	}
	if login.OAuth.ExpiresAt > 0 && login.OAuth.ExpiresAt <= r.ObservedAt.UnixMilli() {
		return r, fmt.Errorf("Claude subscription login expired; open Claude Code to refresh it")
	}
	profile := false
	for _, scope := range login.OAuth.Scopes {
		profile = profile || scope == "user:profile"
	}
	if !profile {
		return r, fmt.Errorf("Claude login does not permit reading subscription usage")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.anthropic.com/api/oauth/usage", nil)
	req.Header.Set("Authorization", "Bearer "+login.OAuth.AccessToken)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("User-Agent", "multica-subscription-capacity/1")
	resp, err := client.Do(req)
	if err != nil {
		return r, fmt.Errorf("Claude usage request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return r, fmt.Errorf("Claude usage request returned HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if err != nil || len(raw) > 65536 {
		return r, fmt.Errorf("Claude usage response unavailable")
	}
	return parseClaudeUsage(raw, r)
}

func parseClaudeUsage(raw []byte, r quota.Report) (quota.Report, error) {
	var windows map[string]json.RawMessage
	if json.Unmarshal(raw, &windows) != nil {
		return r, fmt.Errorf("invalid Claude usage response")
	}
	for _, spec := range []struct {
		id, model string
		minutes   int64
	}{
		{"five_hour", "", 300}, {"seven_day", "", 10080},
		{"seven_day_opus", "Opus", 10080}, {"seven_day_sonnet", "Sonnet", 10080},
	} {
		value := windows[spec.id]
		if len(value) == 0 || string(value) == "null" {
			continue
		}
		var w struct {
			Utilization *float64   `json:"utilization"`
			ResetsAt    *time.Time `json:"resets_at"`
		}
		if json.Unmarshal(value, &w) != nil {
			return r, fmt.Errorf("invalid Claude usage window: %s", spec.id)
		}
		reset := int64(0)
		if w.ResetsAt != nil {
			reset = w.ResetsAt.Unix()
		}
		// The account endpoint uses percentages; stream events use fractions.
		r.Windows = append(r.Windows, quota.Window{ID: spec.id, Model: spec.model, UsedPercent: w.Utilization, ResetsAt: reset, DurationMinutes: spec.minutes, AppliesAll: spec.model == ""})
	}
	if len(r.Windows) > 0 {
		r.Status = "reported"
	}
	if r.Validate(r.ObservedAt) != nil {
		return quota.Report{Provider: r.Provider, Source: r.Source, Status: "unknown", ObservedAt: r.ObservedAt, Windows: []quota.Window{}}, fmt.Errorf("invalid Claude usage values")
	}
	return r, nil
}
