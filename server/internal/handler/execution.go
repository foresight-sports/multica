package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/execution"
	"github.com/multica-ai/multica/server/pkg/runtimecap"
)

func (h *Handler) applyPortableAvailability(ctx context.Context, a db.Agent, response *AgentResponse) {
	if !response.PortableExecution || a.ArchivedAt.Valid || h.TaskService == nil {
		return
	}
	response.RuntimeAvailability = "offline"
	for _, c := range h.TaskService.ExecutionCandidates(ctx, a, execution.ParsePolicy(a.ExecutionPolicy)) {
		if c.Eligible {
			response.RuntimeAvailability = "online"
			break
		}
	}
}

func (h *Handler) GetAgentExecution(w http.ResponseWriter, r *http.Request) {
	a, ok := h.loadAgentForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	raw, err := h.Queries.GetExecutionPolicy(r.Context(), a.ID)
	if err != nil {
		writeError(w, 500, "could not load execution profiles")
		return
	}
	p := execution.ParsePolicy(raw)
	if p.Mode == "" {
		p.Mode = "default"
		p.Preference = "balanced"
		p.Profiles = []execution.Profile{}
	}
	writeJSON(w, 200, p)
}
func (h *Handler) SaveAgentExecution(w http.ResponseWriter, r *http.Request) {
	a, ok := h.loadAgentForUser(w, r, chi.URLParam(r, "id"))
	if !ok || !h.canManageAgent(w, r, a) || !h.canConfigureInstanceAgent(w, r, a) {
		return
	}
	var p execution.Policy
	r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		writeError(w, 400, "invalid execution policy")
		return
	}
	if !h.validateExecutionPolicy(w, r, a, &p) {
		return
	}
	if len(p.Profiles) == 0 {
		active, e := h.Queries.AgentHasProfiledWork(r.Context(), a.ID)
		if e != nil {
			writeError(w, 500, "could not check active tasks")
			return
		}
		if active {
			writeError(w, 409, "finish or cancel profiled tasks before removing all profiles")
			return
		}
	}
	revision := p.Revision
	p.Revision++
	data, _ := json.Marshal(p)
	_, err := h.Queries.SaveExecutionPolicy(r.Context(), db.SaveExecutionPolicyParams{ID: a.ID, Policy: data, ExpectedRevision: revision})
	if err != nil {
		writeError(w, 409, "execution profiles changed; reload before saving")
		return
	}
	writeJSON(w, 200, p)
}
func (h *Handler) PreviewAgentExecution(w http.ResponseWriter, r *http.Request) {
	a, ok := h.loadAgentForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	var req struct {
		Prompt    string            `json:"prompt"`
		Execution execution.Request `json:"execution"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeError(w, 400, "invalid preview request")
		return
	}
	raw, err := h.Queries.GetExecutionPolicy(r.Context(), a.ID)
	if err != nil {
		writeError(w, 500, "could not load profiles")
		return
	}
	p := execution.ParsePolicy(raw)
	candidates := h.TaskService.ExecutionCandidates(r.Context(), a, p)
	profile, reason, selectErr := execution.Select(p, req.Execution, candidates, req.Prompt)
	blocked := ""
	if selectErr != nil {
		blocked = selectErr.Error()
	}
	writeJSON(w, 200, map[string]any{"candidates": candidates, "profile": profile, "reason": reason, "blocked": blocked, "uses_router": p.Mode == "automatic" && p.RouterProfile != ""})
}
func (h *Handler) UpdateTaskExecution(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "taskId"), "task id")
	if !ok {
		return
	}
	task, err := h.Queries.GetAgentTask(r.Context(), id)
	if err != nil {
		writeError(w, 404, "task not found")
		return
	}
	a, ok := h.loadAgentForUser(w, r, uuidToString(task.AgentID))
	if !ok {
		return
	}
	actor, actorID := h.resolveActor(r, requestUserID(r), uuidToString(a.WorkspaceID))
	if !h.canInvokeAgent(r.Context(), a, actor, actorID, h.invokeOriginatorFromRequest(r, actor, actorID), uuidToString(a.WorkspaceID)) {
		writeError(w, 403, "agent invocation is not allowed")
		return
	}
	var req execution.Request
	r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeError(w, 400, "invalid execution request")
		return
	}
	if err := h.TaskService.ValidateExecutionRequest(r.Context(), a, req); err != nil {
		writeError(w, 400, err.Error())
		return
	}

	data, _ := json.Marshal(req)
	policyRaw, e := h.Queries.GetExecutionPolicy(r.Context(), a.ID)
	if e != nil {
		writeError(w, 500, "could not load execution policy")
		return
	}
	next := []byte("{}")
	if len(execution.ParsePolicy(policyRaw).Profiles) > 0 {
		previous := execution.ParseSelection(task.ExecutionSelection)
		next, _ = json.Marshal(execution.Selection{State: "pending", History: previous.History})
	}
	updated, err := h.Queries.SetTaskExecution(r.Context(), db.SetTaskExecutionParams{ID: id, RuntimeID: task.RuntimeID, Selection: next, Request: data, ExpectedSelection: task.ExecutionSelection, FreshSession: req.FreshSession})
	if err != nil {
		writeError(w, 409, "the task has started or changed; create a new run to change execution")
		return
	}
	h.TaskService.NotifyTaskEnqueued(r.Context(), updated)
	writeJSON(w, 200, taskToResponse(updated, uuidToString(a.WorkspaceID)))
}

// Routing proposals are CAS-bound to the exact dispatch. Revalidate every candidate.
func (h *Handler) ResolveTaskExecution(w http.ResponseWriter, r *http.Request) {
	task, ok := h.requireDaemonTaskAccess(w, r, chi.URLParam(r, "taskId"))
	if !ok {
		return
	}
	var req struct {
		RuntimeID         string    `json:"runtime_id"`
		DispatchedAt      time.Time `json:"dispatched_at"`
		ProfileID         string    `json:"profile_id"`
		SelectedRuntimeID string    `json:"selected_runtime_id"`
		Reason            string    `json:"reason"`
		Error             string    `json:"error"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeError(w, 400, "invalid routing proposal")
		return
	}
	runtime, ok := h.requireDaemonRuntimeAccess(w, r, req.RuntimeID)
	if !ok {
		return
	}
	if runtime.ID != task.RuntimeID || !task.DispatchedAt.Time.Equal(req.DispatchedAt) {
		writeError(w, 409, "routing claim changed")
		return
	}
	selection := execution.ParseSelection(task.ExecutionSelection)
	if selection.State != "selecting" {
		writeError(w, 409, "task is not awaiting a routing decision")
		return
	}
	a, err := h.Queries.GetAgent(r.Context(), task.AgentID)
	if err != nil {
		writeError(w, 404, "agent not found")
		return
	}
	raw, err := h.Queries.GetExecutionPolicy(r.Context(), a.ID)
	if err != nil {
		writeError(w, 500, "could not load execution policy")
		return
	}
	p := execution.ParsePolicy(raw)
	candidates := h.TaskService.ConstrainExecutionToTask(r.Context(), task, h.TaskService.ExecutionCandidates(r.Context(), a, p))
	profile, reason, chooseErr := execution.Select(p, execution.Request{ProfileID: req.ProfileID, RuntimeID: req.SelectedRuntimeID}, candidates, selection.Prompt)
	proposalValid := slices.ContainsFunc(selection.Candidates, func(c execution.Profile) bool {
		return c.ID == req.ProfileID && c.Model == profile.Model && c.Provider == profile.Provider && (c.RuntimeID == "" || c.RuntimeID == profile.RuntimeID)
	})
	if !proposalValid || req.ProfileID == "" || req.Error != "" {
		chooseErr = http.ErrAbortHandler
	}
	usedFallback := chooseErr != nil
	if chooseErr != nil && p.AllowFallback {
		profile, reason, chooseErr = execution.Select(p, execution.Request{Mode: "automatic"}, candidates, selection.Prompt)
		reason = "Routing model unavailable or invalid; " + reason
	}
	if chooseErr != nil {
		selection.State = "blocked"
		selection.RoutingFailed = true
		reason = "Routing failed; retry selection or update execution settings"
		profile = selection.Profile
	}
	if !usedFallback && req.Error == "" && req.ProfileID != "" && strings.TrimSpace(req.Reason) != "" {
		reason = "Routing model: " + req.Reason
	}
	if len(reason) > 2000 {
		reason = reason[:2000]
	}
	if !selection.RoutingFailed {
		selection.State = "selected"
	}
	selection.Profile = profile
	selection.Reason = reason
	selection.Candidates = nil
	selection.RuntimeCapabilities = nil
	selection.Prompt = ""
	selection.PolicyRevision = p.Revision
	selection.History = append(selection.History, execution.Decision{ProfileID: profile.ID, RuntimeID: profile.RuntimeID, Model: profile.Model, Reason: reason, At: time.Now().UTC().Format(time.RFC3339)})
	if len(selection.History) > 20 {
		selection.History = selection.History[len(selection.History)-20:]
	}
	data, _ := json.Marshal(selection)
	nextID, ok := parseUUIDOrBadRequest(w, profile.RuntimeID, "selected runtime")
	if !ok {
		return
	}
	updated, err := h.Queries.ResolveTaskExecution(r.Context(), db.ResolveTaskExecutionParams{ID: task.ID, OldRuntimeID: runtime.ID, RuntimeID: nextID, Selection: data, DispatchedAt: task.DispatchedAt, FreshSession: true})
	if err != nil {
		writeError(w, 409, "routing claim changed")
		return
	}
	h.TaskService.NotifyTaskEnqueued(r.Context(), updated)
	writeJSON(w, 200, map[string]string{"status": "queued"})
}
func (h *Handler) ReportExecutionCapabilities(w http.ResponseWriter, r *http.Request) {
	rt, ok := h.requireDaemonRuntimeAccess(w, r, chi.URLParam(r, "runtimeId"))
	if !ok {
		return
	}
	var req struct {
		Inventory                  *runtimecap.Report `json:"runtime_capabilities,omitempty"`
		ObservedAt                 time.Time          `json:"execution_observed_at"`
		OS                         string             `json:"os"`
		Tools                      []string           `json:"tools"`
		Version                    int                `json:"execution_version"`
		Arch                       string             `json:"arch,omitempty"`
		MachineLogsVersion         int                `json:"machine_logs_version"`
		WorkspaceRepositoryVersion int                `json:"workspace_repository_version"`
		WorkHandoffVersion         int                `json:"work_handoff_version"`
		JiraTicketVersion          int                `json:"jira_ticket_version"`
		InstanceUpdateVersion      int                `json:"instance_update_version,omitempty"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
	if json.NewDecoder(r.Body).Decode(&req) != nil || !slices.Contains([]string{"windows", "linux", "darwin"}, req.OS) || len(req.Tools) > 100 || req.Version != 1 || len(req.Arch) > 32 || req.InstanceUpdateVersion < 0 || req.InstanceUpdateVersion > 1 {
		writeError(w, 400, "invalid runtime capabilities")
		return
	}
	req.ObservedAt = time.Now().UTC()
	if req.Inventory != nil {
		if req.Inventory.Validate(rt.Provider) != nil {
			writeError(w, 400, "invalid runtime capability inventory")
			return
		}
		clean := runtimecap.New(rt.Provider)
		clean.Status = req.Inventory.Status
		clean.Truncated = req.Inventory.Truncated
		for _, e := range req.Inventory.Entries {
			clean.Add(e)
		}
		clean.ObservedAt = req.ObservedAt
		req.Inventory = &clean
	}
	data, _ := json.Marshal(req)
	if err := h.Queries.SetExecutionCapabilities(r.Context(), db.SetExecutionCapabilitiesParams{ID: rt.ID, Capabilities: data}); err != nil {
		writeError(w, 500, "could not store runtime capabilities")
		return
	}
	// A machine advertises models even when nobody has opened its picker.
	var catalog struct {
		ObservedAt time.Time `json:"execution_catalog_observed_at"`
	}
	_ = json.Unmarshal(rt.Metadata, &catalog)
	if catalog.ObservedAt.IsZero() || time.Since(catalog.ObservedAt) > time.Hour {
		h.revalidateModelCatalog(r.Context(), uuidToString(rt.ID))
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) GetRuntimeRequiredTools(w http.ResponseWriter, r *http.Request) {
	rt, ok := h.requireDaemonRuntimeAccess(w, r, chi.URLParam(r, "runtimeId"))
	if !ok {
		return
	}
	names, err := h.Queries.ListRuntimeRequiredTools(r.Context(), rt.ID)
	if err != nil {
		writeError(w, 500, "could not load tool requirements")
		return
	}
	if names == nil {
		names = []string{}
	}
	writeJSON(w, 200, names)
}

// All saved profiles describe requirements, never a particular machine.
func (h *Handler) validateExecutionPolicy(w http.ResponseWriter, r *http.Request, a db.Agent, p *execution.Policy) bool {
	runtimes, err := h.Queries.ListAgentRuntimes(r.Context(), a.WorkspaceID)
	if err != nil {
		writeError(w, 500, "could not load model inventory")
		return false
	}
	// Normalize older API clients at the boundary; the stored policy is portable.
	for i := range p.Profiles {
		profile := &p.Profiles[i]
		if profile.RuntimeID != "" {
			found := false
			for _, rt := range runtimes {
				if uuidToString(rt.ID) == profile.RuntimeID && (rt.Visibility == "public" || rt.OwnerID == a.OwnerID) {
					profile.Provider = rt.Provider
					found = true
					break
				}
			}
			if !found {
				writeError(w, 403, "profile catalog is not accessible")
				return false
			}
			profile.RuntimeID = ""
		}
	}
	if err := p.Validate(); err != nil {
		writeError(w, 400, err.Error())
		return false
	}
	for _, profile := range p.Profiles {
		matched := false
		for _, rt := range runtimes {
			if rt.Provider != profile.Provider || (rt.Visibility != "public" && rt.OwnerID != a.OwnerID) {
				continue
			}
			if profile.ID == p.RouterProfile && (rt.ProfileID.Valid || (rt.Provider != "codex" && rt.Provider != "claude")) {
				continue
			}
			catalog := reportedExecutionCatalog(rt.Metadata, time.Now())
			if catalog == nil {
				catalog = h.cachedModelCatalog(r.Context(), uuidToString(rt.ID))
			}
			if catalog == nil {
				continue
			}
			for _, model := range catalog.Models {
				if model.ID != profile.Model {
					continue
				}
				if profile.ThinkingLevel != "" && (model.Thinking == nil || !slices.ContainsFunc(model.Thinking.SupportedLevels, func(v ThinkingLevel) bool { return v.Value == profile.ThinkingLevel })) {
					continue
				}
				if profile.ServiceTier != "" && !slices.ContainsFunc(model.ServiceTiers, func(v ModelServiceTier) bool { return v.ID == profile.ServiceTier }) && !(profile.ServiceTier == "default" && model.SupportsExplicitStandardServiceTier) {
					continue
				}
				data, _ := json.Marshal(catalog.Models)
				if err := h.Queries.SetExecutionCatalog(r.Context(), db.SetExecutionCatalogParams{ID: rt.ID, Models: data}); err != nil {
					writeError(w, 500, "could not save model capabilities")
					return false
				}
				matched = true
			}
		}
		if !matched {
			writeError(w, 400, "No accessible machine advertises the model, reasoning and speed for profile: "+profile.Name+". Refresh model catalogs first.")
			return false
		}
	}
	return true
}
