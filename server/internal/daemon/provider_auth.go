package daemon

import (
	"context"
	_ "embed"
	"time"

	"github.com/multica-ai/multica/server/pkg/protocol"
)

//go:embed provider_auth.ps1
var providerAuthScript []byte

func (d *Daemon) handleProviderAuth(ctx context.Context, runtimeID string, action *protocol.DaemonHeartbeatProviderAuth) {
	status := "failed"
	defer func() {
		reportCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		// Never transmit process output: it can contain OAuth credentials or codes.
		_ = d.client.postJSON(reportCtx, "/api/daemon/runtimes/"+runtimeID+"/provider-auth", map[string]string{"id": action.ID, "status": status}, nil)
	}()
	if action.Provider != "claude" && action.Provider != "codex" {
		return
	}
	entry, ok := d.agents()[action.Provider]
	if !ok || entry.Path == "" {
		return
	}
	if d.tryBeginServerUpdate(ctx) != serverUpdateAcquired {
		return
	}
	defer func() { d.releaseClaimBarrier(); d.updating.Store(false) }()
	setupCtx, cancel := context.WithTimeout(ctx, 14*time.Minute)
	defer cancel()
	if runProviderAuth(setupCtx, action.Provider, entry.Path) == nil {
		status = "completed"
	}
}
