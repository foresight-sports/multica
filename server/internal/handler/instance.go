package handler

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/multica-ai/multica/server/internal/service"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

const maxInstanceInstructions = 100000

type InstanceConfigurationResponse struct {
	Instructions string `json:"instructions"`
	Revision     int64  `json:"revision"`
}

type InstanceAgentResponse struct {
	SourceOwnerID     string `json:"source_owner_id,omitempty"`
	SourceAgentID     string `json:"source_agent_id,omitempty"`
	SourceWorkspaceID string `json:"source_workspace_id,omitempty"`
	ID                string `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	Instructions      string `json:"instructions"`
	Revision          int64  `json:"revision"`
	AgentID           string `json:"agent_id"`
	Enabled           bool   `json:"enabled"`
	RuntimeBound      bool   `json:"runtime_bound"`
}

// Global instructions remain restricted to configured permission managers.
// Human-actor middleware is also installed on every instance route.
func (h *Handler) GetInstanceConfiguration(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUserID(w, r); !ok {
		return
	}
	cfg, err := h.Queries.GetInstanceConfiguration(r.Context())
	if err != nil {
		writeError(w, 500, "could not load instance instructions")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, InstanceConfigurationResponse{cfg.Instructions, cfg.Revision})
}

func (h *Handler) UpdateInstanceConfiguration(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requirePermissionManager(w, r)
	if !ok {
		return
	}
	var req InstanceConfigurationResponse
	if !decodeInstanceBody(w, r, &req) {
		return
	}
	if req.Revision < 1 || utf8.RuneCountInString(req.Instructions) > maxInstanceInstructions {
		writeError(w, 400, "a valid revision and instructions up to 100000 characters are required")
		return
	}
	cfg, err := h.Queries.UpdateInstanceConfiguration(r.Context(), db.UpdateInstanceConfigurationParams{
		Instructions: req.Instructions, UpdatedBy: actor, ExpectedRevision: req.Revision,
	})
	if err != nil {
		instanceWriteError(w, err)
		return
	}
	writeJSON(w, 200, InstanceConfigurationResponse{cfg.Instructions, cfg.Revision})
}

func decodeInstanceBody(w http.ResponseWriter, r *http.Request, dest any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 512*1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		writeError(w, 400, "invalid request body")
		return false
	}
	return true
}

func instanceWriteError(w http.ResponseWriter, err error) {
	var constraint *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, 409, "instance configuration changed or no longer exists; reload before saving")
	case errors.As(err, &constraint) && constraint.Code == "23505":
		writeError(w, 409, "an agent with this name already exists; choose a different name")
	default:
		writeError(w, 500, "could not save instance configuration")
	}
}

// provisionInstanceAgents runs within the caller's transaction. New bindings are
// enabled and workspace-visible, but have no runtime until its owner grants one.
// Re-running this never re-enables an opted-out workspace binding.
func provisionInstanceAgents(ctx context.Context, q *db.Queries, workspaceID pgtype.UUID) ([]db.Agent, error) {
	agents, err := q.ProvisionInstanceAgents(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, a := range agents {
		if err := q.CreateAgentInvocationTarget(ctx, db.CreateAgentInvocationTargetParams{
			AgentID: a.ID, TargetType: "workspace", TargetID: a.WorkspaceID, CreatedBy: a.OwnerID,
		}); err != nil {
			return nil, err
		}
	}
	if err := q.InitializeInstanceAgentSources(ctx); err != nil {
		return nil, err
	}
	if err := q.SyncInstanceAgentConfigurations(ctx); err != nil {
		return nil, err
	}
	for index := range agents {
		fresh, err := q.GetAgent(ctx, agents[index].ID)
		if err != nil {
			return nil, err
		}
		agents[index] = fresh
	}
	return agents, nil
}

// Configuration belongs to one centrally managed source. Workspace members can
// still enable/disable their binding through the existing archive endpoints.
func (h *Handler) canConfigureInstanceAgent(w http.ResponseWriter, r *http.Request, a db.Agent) bool {
	if !a.InstanceAgentID.Valid {
		return true
	}
	if a.InstanceSourceAgentID != a.ID {
		writeError(w, 403, "this instance agent uses its centrally managed runtime, skills and access")
		return false
	}
	actor, _ := h.resolveActor(r, requestUserID(r), uuidToString(a.WorkspaceID))
	if actor != "member" {
		writeError(w, 403, "only human administrators can configure instance agents")
		return false
	}
	return h.requireAgentAction(w, r, permissionEditAgent)
}

// A runtime grant applies only to a task assigned to a linked instance agent.
// The returned workspace owns the task's data, never the runtime's other work.
func (h *Handler) instanceTaskWorkspace(ctx context.Context, task db.AgentTaskQueue) string {
	ws, err := h.Queries.InstanceTaskWorkspace(ctx, db.InstanceTaskWorkspaceParams{AgentID: task.AgentID, RuntimeID: task.RuntimeID})
	if err != nil {
		return ""
	}
	return uuidToString(ws)
}

func (h *Handler) publishInstanceSetup(ctx context.Context, source db.Agent, actorType, actorID string) {
	if !source.InstanceAgentID.Valid || source.InstanceSourceAgentID != source.ID {
		return
	}
	bindings, err := h.Queries.ListInstanceAgentBindings(ctx, source.InstanceAgentID)
	if err != nil {
		return
	}
	for _, a := range bindings {
		if a.ID == source.ID {
			continue
		}
		response := h.agentToResponse(a)
		if h.attachAgentSkills(ctx, &response, a.ID) != nil || h.enrichAgentResponseWithTargets(ctx, &response, a.ID) != nil {
			continue
		}
		h.publish(protocol.EventAgentStatus, uuidToString(a.WorkspaceID), actorType, actorID, map[string]any{"agent": broadcastAgentResponse(response)})
	}
}

func (h *Handler) ensureInstanceAgents(ctx context.Context, workspaceID pgtype.UUID) error {
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Serializes provisioning with central edits and workspace creation. The
	// transaction lock ensures an old definition cannot be inserted after edit.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(71490231)"); err != nil {
		return err
	}
	if _, err = provisionInstanceAgents(ctx, h.Queries.WithTx(tx), workspaceID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (h *Handler) ListInstanceAgents(w http.ResponseWriter, r *http.Request) {
	wsID := h.resolveWorkspaceID(r)
	if _, ok := h.workspaceMember(w, r, wsID); !ok {
		return
	}
	if err := h.ensureInstanceAgents(r.Context(), parseUUID(wsID)); err != nil {
		instanceWriteError(w, err)
		return
	}
	definitions, err := h.Queries.ListInstanceAgents(r.Context())
	if err != nil {
		writeError(w, 500, "could not load instance agents")
		return
	}
	bindings, err := h.Queries.ListAllAgents(r.Context(), parseUUID(wsID))
	if err != nil {
		writeError(w, 500, "could not load workspace agent availability")
		return
	}
	byID := make(map[pgtype.UUID]db.Agent)
	for _, a := range bindings {
		if a.InstanceAgentID.Valid {
			byID[a.InstanceAgentID] = a
		}
	}
	result := make([]InstanceAgentResponse, 0, len(definitions))
	for _, d := range definitions {
		a := byID[d.ID]
		source, err := h.Queries.GetAgent(r.Context(), d.SourceAgentID)
		if err != nil {
			writeError(w, 500, "could not load central agent configuration")
			return
		}
		result = append(result, InstanceAgentResponse{ID: uuidToString(d.ID), Name: d.Name,
			SourceOwnerID: uuidToString(source.OwnerID), SourceAgentID: uuidToString(source.ID), SourceWorkspaceID: uuidToString(source.WorkspaceID),
			Description: d.Description, Instructions: d.Instructions, Revision: d.Revision,
			AgentID: uuidToString(a.ID), Enabled: a.ID.Valid && !a.ArchivedAt.Valid, RuntimeBound: a.RuntimeID.Valid || service.HasExecutionProfiles(a)})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, result)
}

type saveInstanceAgentRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	Revision     int64  `json:"revision"`
}

func (h *Handler) SaveInstanceAgent(w http.ResponseWriter, r *http.Request) {
	action := permissionCreateInstanceAgent
	if r.Method == http.MethodPut {
		action = permissionEditAgent
	}
	if !h.requireAgentAction(w, r, action) {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	actor := parseUUID(userID)
	var req saveInstanceAgentRequest
	if !decodeInstanceBody(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || utf8.RuneCountInString(req.Name) > 100 || utf8.RuneCountInString(req.Description) > maxAgentDescriptionLength || utf8.RuneCountInString(req.Instructions) > maxInstanceInstructions {
		writeError(w, 400, "name is required (up to 100 characters), description up to 255, and instructions up to 100000")
		return
	}
	var id pgtype.UUID
	if r.Method == http.MethodPut {
		id, ok = parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "instance agent id")
		if !ok {
			return
		}
		definition, err := h.Queries.GetInstanceAgent(r.Context(), id)
		if err != nil {
			writeError(w, 404, "instance agent not found")
			return
		}
		user, err := h.Queries.GetUser(r.Context(), actor)
		if err != nil {
			writeError(w, 403, "could not verify account")
			return
		}
		if !emailAllowedForPermission(user.Email, h.cfg.PermissionManagerEmails) {
			source, err := h.Queries.GetAgent(r.Context(), definition.SourceAgentID)
			if err != nil {
				writeError(w, 404, "source agent not found")
				return
			}
			if !h.canManageAgent(w, r, source) {
				return
			}
		}
		if req.Revision < 1 {
			writeError(w, 400, "revision is required")
			return
		}
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		instanceWriteError(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(71490231)"); err != nil {
		instanceWriteError(w, err)
		return
	}
	q := h.Queries.WithTx(tx)
	var saved db.InstanceAgent
	if id.Valid {
		saved, err = q.UpdateInstanceAgent(r.Context(), db.UpdateInstanceAgentParams{
			ID: id, Name: req.Name, Description: req.Description, Instructions: req.Instructions, ExpectedRevision: req.Revision,
		})
	} else {
		saved, err = q.CreateInstanceAgent(r.Context(), db.CreateInstanceAgentParams{
			Name: req.Name, Description: req.Description, Instructions: req.Instructions, CreatedBy: actor,
		})
	}
	if err != nil {
		instanceWriteError(w, err)
		return
	}
	if _, err = provisionInstanceAgents(r.Context(), q, pgtype.UUID{}); err != nil {
		instanceWriteError(w, err)
		return
	}
	changed, err := q.UpdateInstanceAgentBindings(r.Context(), db.UpdateInstanceAgentBindingsParams{
		InstanceAgentID: saved.ID, Name: pgtype.Text{String: saved.Name, Valid: true}, Description: saved.Description, Instructions: saved.Instructions,
	})
	if err != nil {
		instanceWriteError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		instanceWriteError(w, err)
		return
	}
	for _, a := range changed {
		event := protocol.EventAgentStatus
		if !id.Valid {
			event = protocol.EventAgentCreated
		}
		h.publish(event, uuidToString(a.WorkspaceID), "member", uuidToString(actor), map[string]any{"agent": broadcastAgentResponse(h.agentToResponse(a))})
	}
	status := http.StatusOK
	if !id.Valid {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]string{"id": uuidToString(saved.ID)})
}

func composeInstanceInstructions(instance, agent string) string {
	instance = strings.TrimSpace(instance)
	if instance == "" {
		return agent
	}
	return "## Instance instructions\n\nThese instructions apply to every agent in every workspace on this Multica instance. Workspace and agent instructions refine them; when they conflict, follow the instance instructions.\n\n" + instance + "\n\n## Agent instructions\n\n" + agent
}
