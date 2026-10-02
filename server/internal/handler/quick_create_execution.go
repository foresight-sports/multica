package handler

import (
	"context"
	"net/http"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/execution"
)

// Portable agents have no saved runtime ID. Check eligible machines without
// binding the agent; routing and start repeat the same feature constraints.
func (h *Handler) checkPortableQuickCreate(ctx context.Context, a db.Agent, fields, sourceContext bool) (int, map[string]any) {
	raw, err := h.Queries.GetExecutionPolicy(ctx, a.ID)
	if err != nil {
		return http.StatusInternalServerError, map[string]any{"error": "failed to load agent execution profiles"}
	}
	p := execution.ParsePolicy(raw)
	candidates := h.TaskService.QuickCreateExecutionCandidates(ctx, h.TaskService.ExecutionCandidates(ctx, a, p), fields, sourceContext)
	if _, _, err := execution.Select(p, execution.Request{}, candidates, ""); err != nil {
		return http.StatusUnprocessableEntity, map[string]any{"code": "agent_unavailable", "reason": "No eligible machine is ready to create this issue; check workspace readiness, execution profiles and daemon updates"}
	}
	return 0, nil
}
