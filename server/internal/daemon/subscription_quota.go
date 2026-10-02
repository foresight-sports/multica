package daemon

import (
	"context"
	"github.com/multica-ai/multica/server/pkg/agent"
	"github.com/multica-ai/multica/server/pkg/quota"
	"time"
)

// Built-in workspace bindings on this daemon share the CLI login. Custom
// profiles and per-agent environment overrides are deliberately excluded.
func (d *Daemon) reportSubscriptionQuota(ctx context.Context, r quota.Report) {
	for _, id := range d.allRuntimeIDs() {
		rt := d.findRuntime(id)
		if rt == nil || rt.Provider != r.Provider || rt.ProfileID != "" {
			continue
		}
		postCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_ = d.client.postJSON(postCtx, "/api/daemon/runtimes/"+id+"/subscription-quota", r, nil)
		cancel()
	}
}
func (d *Daemon) subscriptionQuotaLoop(ctx context.Context) {
	timer := time.NewTicker(2 * time.Minute)
	defer timer.Stop()
	var nextClaudePoll time.Time
	poll := func() {
		if _, ok := d.agents()["claude"]; ok && len(defaultArgsForProvider(d.cfg, "claude")) == 0 && !time.Now().Before(nextClaudePoll) {
			report, err := agent.ReadClaudeQuota(ctx)
			if err != nil && ctx.Err() == nil {
				nextClaudePoll = time.Now().Add(10 * time.Minute)
				d.logger.Warn("Claude subscription capacity unavailable; retrying in 10 minutes", "reason", err)
			}
			if ctx.Err() == nil {
				d.reportSubscriptionQuota(ctx, report)
			}
		}
		if entry, ok := d.agents()["codex"]; ok {
			entry, _ = d.resolveAgentEntry(ctx, "codex", entry)
			report := quota.Report{Provider: "codex", Status: "unknown", Source: "codex_app_server", ObservedAt: time.Now().UTC(), Windows: []quota.Window{}}
			var err error
			if len(defaultArgsForProvider(d.cfg, "codex")) == 0 {
				report, err = agent.ReadCodexQuota(ctx, agent.NewCommand(entry.Path, nil))
			}
			if err != nil {
				report = quota.Report{Provider: "codex", Status: "unknown", Source: "codex_app_server", ObservedAt: time.Now().UTC(), Windows: []quota.Window{}}
			}
			if ctx.Err() == nil {
				d.reportSubscriptionQuota(ctx, report)
			}
		}
	}
	poll()
	for {
		select {
		case <-ctx.Done():
			return
		case report := <-d.subscriptionQuotaReports:
			d.reportSubscriptionQuota(ctx, report)
		case <-timer.C:
			poll()
		}
	}
}
