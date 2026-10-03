package daemon

import (
	"encoding/json"
	"github.com/multica-ai/multica/server/pkg/runtimecap"
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeCapabilitiesExcludeProjectInstallsAndCachedVersions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := filepath.Join(home, ".claude")
	os.MkdirAll(filepath.Join(root, "plugins"), 0700)
	install := filepath.Join(home, "plugin")
	os.MkdirAll(filepath.Join(install, ".claude-plugin"), 0700)
	os.WriteFile(filepath.Join(install, ".claude-plugin", "plugin.json"), []byte(`{"name":"designer","description":"Design screens","mcpServers":{"secret":{"env":{"TOKEN":"private"}}}}`), 0600)
	data, _ := json.Marshal(map[string]any{"plugins": map[string]any{"designer@market": []any{map[string]any{"scope": "user", "installPath": install}}, "project@market": []any{map[string]any{"scope": "project", "installPath": install}}}})
	os.WriteFile(filepath.Join(root, "plugins", "installed_plugins.json"), data, 0600)
	os.WriteFile(filepath.Join(root, "settings.json"), []byte(`{"enabledPlugins":{"designer@market":false,"project@market":true,"cache-only@market":true}}`), 0600)
	r := runtimecap.New("claude")
	appendClaudeCapabilities(&r)
	if len(r.Entries) != 1 || r.Entries[0].Enabled == nil || *r.Entries[0].Enabled || r.Entries[0].Auth != "unknown" {
		t.Fatal("scope/enablement changed", r)
	}
}
