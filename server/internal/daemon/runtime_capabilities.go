package daemon

import (
	"context"
	"encoding/json"
	"github.com/multica-ai/multica/server/pkg/agent"
	"github.com/multica-ai/multica/server/pkg/runtimecap"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Read local configuration only. MCP credentials, commands, URLs and paths
// never enter the report; configured servers have unknown authentication.
func (d *Daemon) discoverRuntimeCapabilities(ctx context.Context, rt Runtime) runtimecap.Report {
	r := runtimecap.New(rt.Provider)
	if rt.ProfileID != "" || len(defaultArgsForProvider(d.cfg, rt.Provider)) > 0 || (rt.Provider == "claude" && os.Getenv("CLAUDE_CONFIG_DIR") != "") {
		r.Status = "unsupported"
		return r
	}
	if rt.Provider == "codex" {
		if entry, ok := d.agents()[rt.Provider]; ok {
			if resolved, _ := d.resolveAgentEntry(ctx, rt.Provider, entry); resolved.Path != "" {
				observed, err := agent.ReadCodexCapabilities(ctx, agent.NewCommand(resolved.Path, nil))
				if err == nil {
					r = observed
				}
			}
		}
	} else {
		skills, supported, err := listRuntimeLocalSkills(rt.Provider)
		if !supported {
			r.Status = "unsupported"
		} else if err == nil {
			r.Status = "reported"
			for _, s := range skills {
				if s.Plugin == "" {
					r.Add(runtimecap.Entry{Kind: "skill", Name: s.Name, Description: s.Description})
				}
			}
		}
		if rt.Provider == "claude" {
			appendClaudeCapabilities(&r)
		}
	}
	servers, supported, err := listRuntimeLocalMcpServers(rt.Provider)
	if supported && err == nil {
		for _, s := range servers {
			r.Add(runtimecap.Entry{Kind: "mcp", Name: s.Name, Enabled: runtimecap.Bool(s.Enabled)})
		}
	}
	r.ObservedAt = time.Now().UTC()
	return r
}
func boundedCapabilityFile(path string) []byte {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil
	}
	b, _ := os.ReadFile(path)
	return b
}
func appendClaudeCapabilities(r *runtimecap.Report) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	var installed claudeInstalledPluginsFile
	if json.Unmarshal(boundedCapabilityFile(filepath.Join(home, ".claude", "plugins", "installed_plugins.json")), &installed) != nil {
		return
	}
	var settings claudeSettingsFile
	_ = json.Unmarshal(boundedCapabilityFile(filepath.Join(home, ".claude", "settings.json")), &settings)
	ids := []string{}
	for id := range installed.Plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		// Project installs must never be advertised to another workspace.
		for _, install := range installed.Plugins[id] {
			if install.Scope != "user" {
				continue
			}
			var manifest struct {
				Name        string
				Description string
			}
			if json.Unmarshal(boundedCapabilityFile(filepath.Join(install.InstallPath, ".claude-plugin", "plugin.json")), &manifest) != nil {
				continue
			}
			var enabled *bool
			if v, ok := settings.EnabledPlugins[id]; ok {
				enabled = runtimecap.Bool(v)
			}
			r.Add(runtimecap.Entry{Kind: "plugin", Name: id, Description: manifest.Description, Enabled: enabled})
			break
		}
	}
}
