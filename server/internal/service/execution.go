package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/multica-ai/multica/server/pkg/workspacerepo"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	agentpkg "github.com/multica-ai/multica/server/pkg/agent"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/execution"
	"github.com/multica-ai/multica/server/pkg/protocol"
	"github.com/multica-ai/multica/server/pkg/quota"
)

type executionRequestKey struct{}

func WithExecutionRequest(ctx context.Context, req execution.Request) context.Context {
	return context.WithValue(ctx, executionRequestKey{}, req)
}
func taskExecutionRequest(ctx context.Context) []byte {
	req, _ := ctx.Value(executionRequestKey{}).(execution.Request)
	b, _ := json.Marshal(req)
	return b
}

func (s *TaskService) ExecutionCandidates(ctx context.Context, a db.Agent, p execution.Policy) []execution.Candidate {
	return s.executionCandidates(ctx, a, p, true)
}
func (s *TaskService) executionCandidates(ctx context.Context, a db.Agent, p execution.Policy, checkRepository bool) []execution.Candidate {
	result := make([]execution.Candidate, 0, len(p.Profiles))
	runtimes, listErr := s.Queries.ListAgentExecutionRuntimes(ctx, a.ID)
	if listErr != nil {
		for _, profile := range p.Profiles {
			result = append(result, execution.Candidate{Profile: profile, Reason: "Machine inventory unavailable"})
		}
		return result
	}
	for _, requirement := range p.Profiles {
		matches := 0
		for _, runtime := range runtimes {
			if runtime.Provider != requirement.Provider {
				continue
			}
			matches++
			profile := requirement
			profile.RuntimeID = util.UUIDToString(runtime.ID)
			c := execution.Candidate{Profile: profile, Eligible: true}
			// The inventory query has already applied the source/binding access gate.
			rt := runtime
			if rt.Status != "online" || !rt.LastSeenAt.Valid || time.Since(rt.LastSeenAt.Time) > time.Duration(RuntimeClaimFreshnessSeconds)*time.Second {
				c.Eligible = false
				c.Reason = "Runtime is offline or stale"
			}
			if c.Eligible && checkRepository {
				status, e := workspacerepo.Status(ctx, s.Queries, rt, util.UUIDToString(a.WorkspaceID))
				if e != nil || !status.Usable {
					c.Eligible = false
					c.Reason = "Workspace repository is not ready"
					if e == nil {
						c.Reason = status.Message
					}
				}
			}
			var metadata struct {
				ObservedAt time.Time    `json:"execution_observed_at"`
				Quota      quota.Report `json:"subscription_quota"`
				Version    int          `json:"execution_version"`
				OS         string       `json:"os"`
				Tools      []string     `json:"tools"`
				Models     []struct {
					ID       string `json:"id"`
					Thinking *struct {
						Levels []struct {
							Value string `json:"value"`
						} `json:"supported_levels"`
					} `json:"thinking"`
					ServiceTiers []struct {
						ID string `json:"id"`
					} `json:"service_tiers"`
					Standard bool `json:"supports_explicit_standard_service_tier"`
				} `json:"execution_models"`
			}
			_ = json.Unmarshal(rt.Metadata, &metadata)
			if len(profile.RequiredTools) > 0 && (metadata.ObservedAt.IsZero() || time.Since(metadata.ObservedAt) > 5*time.Minute) {
				c.Eligible = false
				c.Reason = "Tool inventory is stale; update or reconnect this machine"
			}
			if profile.ID == p.RouterProfile && (rt.ProfileID.Valid || (rt.Provider != "codex" && rt.Provider != "claude")) {
				c.Eligible = false
				c.Reason = "Task understanding requires a built-in Codex or Claude runtime"
			}
			if raw, err := s.Queries.GetSubscriptionQuota(ctx, rt.ID); err == nil {
				metadata.Quota = quota.Parse(raw)
			}
			var env map[string]string
			var args []string
			_ = json.Unmarshal(a.CustomEnv, &env)
			_ = json.Unmarshal(a.CustomArgs, &args)
			if !rt.ProfileID.Valid && len(env) == 0 && len(args) == 0 && metadata.Quota.Blocks(profile.Model, time.Now()) {
				c.Eligible = false
				c.Reason = "Subscription capacity exhausted"
			}
			if profile.RequiredOS != "" && metadata.OS != profile.RequiredOS {
				c.Eligible = false
				c.Reason = "Operating system requirement not met"
			}
			for _, tool := range profile.RequiredTools {
				if !slices.Contains(metadata.Tools, tool) {
					c.Eligible = false
					c.Reason = "Required tool unavailable: " + tool
				}
			}
			if metadata.Version < 1 {
				c.Eligible = false
				c.Reason = "Update the daemon to support execution profiles"
			}
			modelOK := false
			for _, m := range metadata.Models {
				if m.ID != profile.Model {
					continue
				}
				thinkingOK := profile.ThinkingLevel == ""
				if m.Thinking != nil {
					for _, level := range m.Thinking.Levels {
						if level.Value == profile.ThinkingLevel {
							thinkingOK = true
						}
					}
				}
				tierOK := profile.ServiceTier == "" || (profile.ServiceTier == "default" && m.Standard)
				for _, tier := range m.ServiceTiers {
					if tier.ID == profile.ServiceTier {
						tierOK = true
					}
				}
				modelOK = thinkingOK && tierOK
			}
			if !modelOK {
				c.Eligible = false
				c.Reason = "Model, reasoning or speed is not available on this machine"
			}
			result = append(result, c)
		}
		if matches == 0 {
			result = append(result, execution.Candidate{Profile: requirement, Reason: "No accessible machine advertises this provider"})
		}
	}
	return result
}

// Route queued work before claim caches. CAS makes concurrent daemon polls harmless.
func (s *TaskService) RouteExecutionTasks(ctx context.Context, runtimeIDs []pgtype.UUID) error {
	ids := []string{}
	for _, id := range runtimeIDs {
		ids = append(ids, util.UUIDToString(id))
	}
	tasks, err := s.Queries.ListExecutionRoutingTasks(ctx, ids)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		a, err := s.Queries.GetAgent(ctx, task.AgentID)
		if err != nil {
			return err
		}
		raw, err := s.Queries.GetExecutionPolicy(ctx, a.ID)
		if err != nil {
			return err
		}
		p := execution.ParsePolicy(raw)
		old := execution.ParseSelection(task.ExecutionSelection)
		req := execution.ParseRequest(task.ExecutionRequest)
		if old.RoutingFailed && old.PolicyRevision == p.Revision {
			continue
		}
		if old.Locked {
			continue
		}
		unconstrained, workflowErr := s.Queries.TaskWorkflowAllowed(ctx, db.TaskWorkflowAllowedParams{AgentID: task.AgentID, IssueID: task.IssueID})
		if workflowErr != nil {
			return workflowErr
		}
		machineSetup := !unconstrained
		routingCandidates := s.ExecutionCandidates(ctx, a, p)
		candidates := s.ConstrainExecutionToTask(ctx, task, append([]execution.Candidate{}, routingCandidates...))
		if old.State == "selected" || old.State == "selecting" {
			oldCandidates := candidates
			if old.State == "selecting" && !machineSetup {
				oldCandidates = routingCandidates
			}
			if slices.ContainsFunc(oldCandidates, func(c execution.Candidate) bool {
				return c.Eligible && c.Profile.ID == old.Profile.ID && c.Profile.RuntimeID == old.Profile.RuntimeID && c.Profile.Model == old.Profile.Model && c.Profile.ThinkingLevel == old.Profile.ThinkingLevel && c.Profile.ServiceTier == old.Profile.ServiceTier
			}) {
				continue
			}
			// Explicit and already-started work never silently moves to another runtime.
			if req.RuntimeID != "" {
				blocked := old
				blocked.State = "blocked"
				blocked.Reason = "Selected execution profile is unavailable"
				data, _ := json.Marshal(blocked)
				_, e := s.Queries.SetTaskExecution(ctx, db.SetTaskExecutionParams{ID: task.ID, RuntimeID: task.RuntimeID, Selection: data, Request: task.ExecutionRequest, ExpectedSelection: task.ExecutionSelection, FreshSession: false})
				if e != nil {
					return e
				}
				continue
			}
		}
		prompt := task.TriggerSummary.String
		if len(task.Context) > 0 {
			var quick struct {
				Prompt string `json:"prompt"`
			}
			if json.Unmarshal(task.Context, &quick) == nil {
				prompt += "\n" + quick.Prompt
			}
		}
		if task.IssueID.Valid {
			if issue, e := s.Queries.GetIssue(ctx, task.IssueID); e == nil {
				prompt = issue.Title + "\n" + issue.Description.String + "\n" + prompt
			}
		}
		if task.ChatInputTaskID.Valid {
			if messages, e := s.Queries.ListChatInputMessages(ctx, task.ChatInputTaskID); e == nil {
				for _, m := range messages {
					prompt += "\n" + m.Content
				}
			}
		}
		if task.AutopilotRunID.Valid {
			if run, e := s.Queries.GetAutopilotRun(ctx, task.AutopilotRunID); e == nil {
				if ap, e := s.Queries.GetAutopilot(ctx, run.AutopilotID); e == nil {
					prompt += "\n" + ap.Title + "\n" + ap.Description.String
				}
			}
		}
		if len(prompt) > 32000 {
			prompt = prompt[:32000]
		}
		selectionRequest := req
		if old.State != "selecting" && old.Profile.ID != "" && (old.Explicit || !p.AllowFallback) {
			selectionRequest.ProfileID = old.Profile.ID
		}
		profile, reason, chooseErr := execution.Select(p, selectionRequest, candidates, prompt)
		selected := execution.Selection{State: "selected", Profile: profile, PolicyRevision: p.Revision, Reason: reason, History: old.History, Explicit: req.ProfileID != "" || req.RuntimeID != "" || req.Model != ""}
		fresh := true
		if chooseErr != nil {
			selected.State = "blocked"
			if old.Profile.ID != "" && (old.Explicit || !p.AllowFallback) {
				selected.Profile = old.Profile
				selected.Explicit = old.Explicit
			}
			selected.Reason = chooseErr.Error()
		} else {
			// Keep the last started conversation on its chosen runtime/model unless explicitly changed.
			if !req.FreshSession && !task.ForceFreshSession && !selected.Explicit {
				prior, e := s.Queries.GetPreviousExecution(ctx, db.GetPreviousExecutionParams{AgentID: a.ID, ID: task.ID, IssueID: task.IssueID, ChatSessionID: task.ChatSessionID})
				if e == nil {
					prev := execution.ParseSelection(prior.ExecutionSelection)
					if slices.ContainsFunc(candidates, func(c execution.Candidate) bool {
						return c.Eligible && c.Profile.ID == prev.Profile.ID && c.Profile.Model == prev.Profile.Model && c.Profile.RuntimeID == prev.Profile.RuntimeID
					}) {
						for _, candidate := range candidates {
							if candidate.Eligible && candidate.Profile.ID == prev.Profile.ID && candidate.Profile.RuntimeID == prev.Profile.RuntimeID && candidate.Profile.Model == prev.Profile.Model {
								selected.Profile = candidate.Profile
								break
							}
						}
						selected.Reason = "Pinned to the existing conversation"
						fresh = false
					} else {
						fresh = true
					}
				}
			}
			if selected.Explicit {
				fresh = true
			}
			if !machineSetup && selected.Reason != "Pinned to the existing conversation" && selectionRequest.ProfileID == "" && !selected.Explicit && req.Mode != "default" && (req.Mode == "automatic" || p.Mode == "automatic") && p.RouterProfile != "" && old.State != "selecting" {
				seen := map[string]bool{}
				for _, c := range candidates {
					if c.Eligible && !seen[c.Profile.ID] {
						seen[c.Profile.ID] = true
						requirement := c.Profile
						requirement.RuntimeID = ""
						selected.Candidates = append(selected.Candidates, requirement)
					}
				}
				for _, c := range routingCandidates {
					if c.Eligible && c.Profile.ID == p.RouterProfile {
						selected.Profile = c.Profile
						selected.State = "selecting"
						break
					}
				}
				if selected.State != "selecting" && !p.AllowFallback {
					selected.State = "blocked"
					selected.Reason = "Routing profile is unavailable; fallback is disabled"
				}
				if selected.State == "selecting" {
					selected.Prompt = "Selection preference: " + p.Preference + "\n" + prompt
					selected.Reason = "Selecting a model profile; an eligible machine will be assigned automatically"
				}
			}
			if old.Profile.RuntimeID != "" && (old.Profile.RuntimeID != selected.Profile.RuntimeID || old.Profile.Model != selected.Profile.Model) {
				fresh = true
			}
		}
		if old.State == selected.State && old.Reason == selected.Reason && old.PolicyRevision == p.Revision && old.Profile.ID == selected.Profile.ID && old.Profile.RuntimeID == selected.Profile.RuntimeID && old.Profile.Model == selected.Profile.Model && old.Profile.ThinkingLevel == selected.Profile.ThinkingLevel && old.Profile.ServiceTier == selected.Profile.ServiceTier {
			continue
		}
		selected.History = append(selected.History, execution.Decision{ProfileID: selected.Profile.ID, RuntimeID: selected.Profile.RuntimeID, Model: selected.Profile.Model, Reason: selected.Reason, At: time.Now().UTC().Format(time.RFC3339)})
		if len(selected.History) > 20 {
			selected.History = selected.History[len(selected.History)-20:]
		}
		nextID := task.RuntimeID
		if selected.Profile.RuntimeID != "" {
			nextID, _ = util.ParseUUID(selected.Profile.RuntimeID)
		}
		data, _ := json.Marshal(selected)
		updated, e := s.Queries.SetTaskExecution(ctx, db.SetTaskExecutionParams{ID: task.ID, RuntimeID: nextID, Selection: data, Request: task.ExecutionRequest, ExpectedSelection: task.ExecutionSelection, FreshSession: fresh})
		if e == nil {
			s.NotifyTaskEnqueued(ctx, updated)
		}
	}
	return nil
}

func (s *TaskService) ValidateExecutionRequest(ctx context.Context, a db.Agent, req execution.Request) error {
	if len(req.Instruction) > 8000 {
		return fmt.Errorf("continuation instruction must be at most 8000 bytes")
	}
	if req.Mode != "" && req.Mode != "default" && req.Mode != "automatic" {
		return fmt.Errorf("invalid execution mode")
	}
	raw, err := s.Queries.GetExecutionPolicy(ctx, a.ID)
	if err != nil {
		return err
	}
	p := execution.ParsePolicy(raw)
	if len(p.Profiles) == 0 {
		if req.ProfileID != "" || req.RuntimeID != "" || req.Model != "" {
			return fmt.Errorf("configure approved execution profiles first")
		}
		return nil
	}
	candidates := []execution.Candidate{}
	if req.RuntimeID != "" {
		candidates = s.executionCandidates(ctx, a, p, false)
	} else {
		for _, profile := range p.Profiles {
			candidates = append(candidates, execution.Candidate{Profile: profile, Eligible: true})
		}
	}
	_, _, err = execution.Select(p, req, candidates, "")
	return err
}

// A local-only project must run on a machine with its directory binding.
func (s *TaskService) ConstrainExecutionToTask(ctx context.Context, task db.AgentTaskQueue, candidates []execution.Candidate) []execution.Candidate {
	for i := range candidates {
		rid, err := util.ParseUUID(candidates[i].Profile.RuntimeID)
		if err != nil {
			candidates[i].Eligible = false
			continue
		}
		allowed, err := s.Queries.TaskWorkflowAllowed(ctx, db.TaskWorkflowAllowedParams{AgentID: task.AgentID, IssueID: task.IssueID, RuntimeID: rid})
		if err != nil || !allowed {
			candidates[i].Eligible = false
			candidates[i].Reason = "Waiting for installation approval or its original machine"
		}
	}
	var quickCreate QuickCreateContext
	if json.Unmarshal(task.Context, &quickCreate) == nil && quickCreate.Type == QuickCreateContextType {
		candidates = s.QuickCreateExecutionCandidates(ctx, candidates, quickCreate.Priority != "" || quickCreate.DueDate != "", quickCreate.SourceContextID != "")
	}
	var projectID, workspaceID pgtype.UUID
	block := func(reason string) []execution.Candidate {
		for i := range candidates {
			candidates[i].Eligible = false
			candidates[i].Reason = reason
		}
		return candidates
	}
	allowed, jiraErr := s.Queries.TaskJiraAllowed(ctx, db.TaskJiraAllowedParams{Column1: task.IssueID, Column2: task.AgentID, Column3: task.Context})
	if jiraErr != nil {
		return block("JIRA requirement could not be checked")
	}
	if !allowed {
		return block("JIRA ticket ID is required before agent work can start")
	}
	a, e := s.Queries.GetAgent(ctx, task.AgentID)
	if e != nil {
		return block("Agent context unavailable")
	}
	workspaceID = a.WorkspaceID
	switch {
	case task.IssueID.Valid:
		issue, e := s.Queries.GetIssue(ctx, task.IssueID)
		if e != nil || issue.WorkspaceID != workspaceID {
			return block("Task context unavailable")
		}
		projectID = issue.ProjectID
		jiraKey, jiraErr := s.Queries.GetIssueJiraKey(ctx, issue.ID)
		if jiraErr != nil {
			return block("JIRA ticket context unavailable")
		}
		if jiraKey != "" {
			for i := range candidates {
				runtimeID, parseErr := util.ParseUUID(candidates[i].Profile.RuntimeID)
				if parseErr != nil {
					candidates[i].Eligible = false
					candidates[i].Reason = "Invalid runtime identity"
					continue
				}
				rt, e := s.Queries.GetAgentRuntime(ctx, runtimeID)
				var metadata struct {
					Version int `json:"jira_ticket_version"`
				}
				if e == nil {
					_ = json.Unmarshal(rt.Metadata, &metadata)
				}
				if e != nil || metadata.Version < 1 {
					candidates[i].Eligible = false
					candidates[i].Reason = "Update this machine to support JIRA ticket context"
				}
			}
		}
	case task.ChatSessionID.Valid:
		chat, e := s.Queries.GetChatSession(ctx, task.ChatSessionID)
		if e != nil || chat.WorkspaceID != workspaceID {
			return block("Chat context unavailable")
		}
		projectID = chat.ProjectID
	case task.AutopilotRunID.Valid:
		run, e := s.Queries.GetAutopilotRun(ctx, task.AutopilotRunID)
		if e != nil {
			return block("Autopilot unavailable")
		}
		ap, e := s.Queries.GetAutopilot(ctx, run.AutopilotID)
		if e != nil || ap.WorkspaceID != workspaceID {
			return block("Autopilot context unavailable")
		}
		projectID = ap.ProjectID
	default:
		var quick QuickCreateContext
		if json.Unmarshal(task.Context, &quick) == nil && quick.ProjectID != "" {
			projectID, e = util.ParseUUID(quick.ProjectID)
			if e != nil {
				return block("Invalid project context")
			}
		}
	}
	// Workspace repository preparation is authoritative over legacy per-project
	// directory bindings; every eligible machine has already verified this path.
	config, _, configErr := workspacerepo.Load(ctx, s.Queries, "workspace", util.UUIDToString(workspaceID))
	if configErr != nil {
		return block("Workspace repository configuration unavailable")
	}
	if config.Folder != "" || !projectID.Valid {
		return candidates
	}
	resources, e := s.Queries.ListProjectResourcesInWorkspace(ctx, db.ListProjectResourcesInWorkspaceParams{ProjectID: projectID, WorkspaceID: workspaceID})
	if e != nil {
		for i := range candidates {
			candidates[i].Eligible = false
			candidates[i].Reason = "Project resources are unavailable"
		}
		return candidates
	}
	hosts := map[string]bool{}
	hasRepo := false
	for _, r := range resources {
		if r.ResourceType == "github_repo" {
			hasRepo = true
		}
		if r.ResourceType == "local_directory" {
			var ref struct {
				DaemonID string `json:"daemon_id"`
			}
			if json.Unmarshal(r.ResourceRef, &ref) == nil {
				hosts[ref.DaemonID] = true
			}
		}
	}
	if len(hosts) == 0 || hasRepo {
		return candidates
	}
	for i := range candidates {
		id, e := util.ParseUUID(candidates[i].Profile.RuntimeID)
		if e != nil {
			continue
		}
		rt, e := s.Queries.GetAgentRuntime(ctx, id)
		if e != nil || !hosts[rt.DaemonID.String] {
			candidates[i].Eligible = false
			candidates[i].Reason = "Project directory is on another machine"
		}
	}
	return candidates
}

// Apply feature gates to the machine chosen for a portable quick-create task,
// both at admission and again during routing/start validation.
func (s *TaskService) QuickCreateExecutionCandidates(ctx context.Context, candidates []execution.Candidate, fields, sourceContext bool) []execution.Candidate {
	minimum := agentpkg.MinQuickCreateCLIVersion
	if fields {
		minimum = agentpkg.MinQuickCreateFieldsCLIVersion
	}
	for i := range candidates {
		c := &candidates[i]
		if !c.Eligible {
			continue
		}
		id, err := util.ParseUUID(c.Profile.RuntimeID)
		if err != nil {
			c.Eligible = false
			c.Reason = "Machine unavailable"
			continue
		}
		rt, err := s.Queries.GetAgentRuntime(ctx, id)
		var metadata struct {
			Version      string   `json:"cli_version"`
			Capabilities []string `json:"capabilities"`
		}
		if err != nil || json.Unmarshal(rt.Metadata, &metadata) != nil || agentpkg.CheckMinCLIVersionFor(metadata.Version, minimum) != nil {
			c.Eligible = false
			c.Reason = "Update this machine to support issue creation"
			continue
		}
		if sourceContext && !slices.Contains(metadata.Capabilities, protocol.DaemonCapabilitySourceContextQuickCreateV1) {
			c.Eligible = false
			c.Reason = "Update this machine to support captured context"
		}
	}
	return candidates
}

// Configured portable agents may queue while no machine is ready.
func HasExecutionProfiles(a db.Agent) bool {
	return len(execution.ParsePolicy(a.ExecutionPolicy).Profiles) > 0
}
