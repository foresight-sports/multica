package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// An absent project key inherits the workspace default; an empty value disables intake.
type IssueIntakeConfig struct {
	Revision       int32             `json:"revision"`
	DefaultSquadID string            `json:"default_squad_id"`
	Projects       map[string]string `json:"projects"`
}

func ParseIssueIntake(raw []byte) (IssueIntakeConfig, error) {
	c := IssueIntakeConfig{Projects: map[string]string{}}
	e := json.Unmarshal(raw, &c)
	if c.Projects == nil {
		c.Projects = map[string]string{}
	}
	return c, e
}
func (c IssueIntakeConfig) SquadFor(projectID string) string {
	if id, ok := c.Projects[projectID]; ok {
		return id
	}
	return c.DefaultSquadID
}

var ErrIssueIntakeUnavailable = errors.New("automatic intake squad is unavailable; ask a workspace administrator to check intake settings, or explicitly assign the ticket")

// Workspace-wide intake requires a workspace invocation grant, including after
// configuration changes. It never borrows an administrator's private-agent access.
func ValidateIssueIntakeSquad(ctx context.Context, q *db.Queries, ws, id pgtype.UUID) error {
	squad, e := q.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: id, WorkspaceID: ws})
	if e != nil || squad.ArchivedAt.Valid {
		return ErrIssueIntakeUnavailable
	}
	a, e := q.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: squad.LeaderID, WorkspaceID: ws})
	if e != nil || a.ArchivedAt.Valid || (!a.RuntimeID.Valid && !HasExecutionProfiles(a)) || a.PermissionMode != "public_to" {
		return ErrIssueIntakeUnavailable
	}
	targets, e := q.ListAgentInvocationTargets(ctx, a.ID)
	if e != nil {
		return e
	}
	for _, target := range targets {
		if target.TargetType == "workspace" {
			return nil
		}
	}
	return ErrIssueIntakeUnavailable
}

// ResolveIssueIntake applies the shared read-only routing policy to a candidate.
// Callers must validate project and parent scope before calling.
func (s *IssueService) ResolveIssueIntake(ctx context.Context, q *db.Queries, p *IssueCreateParams, projectID pgtype.UUID) error {
	// Human-requested quick-create remains intake even though its author is an agent.
	// Trust the persisted task and authenticated creator, never creator UI fields.
	if p.CreatorType == "agent" && p.OriginID.Valid && !p.ParentIssueID.Valid {
		task, err := q.GetAgentTaskInWorkspace(ctx, db.GetAgentTaskInWorkspaceParams{ID: p.OriginID, WorkspaceID: p.WorkspaceID})
		if err == nil && task.AgentID == p.CreatorID {
			var qc QuickCreateContext
			if json.Unmarshal(task.Context, &qc) == nil && qc.Type == QuickCreateContextType && qc.ParentIssueID == "" && qc.WorkspaceID == util.UUIDToString(p.WorkspaceID) && qc.RequesterID == util.UUIDToString(task.OriginatorUserID) && task.OriginatorUserID.Valid {
				candidate := *p
				candidate.CreatorType = "member"
				candidate.CreatorID = task.OriginatorUserID
				candidate.AssigneeType = pgtype.Text{}
				candidate.AssigneeID = pgtype.UUID{}
				if err := s.ResolveIssueIntake(ctx, q, &candidate, projectID); err != nil {
					return err
				}
				if candidate.intakeRouted {
					p.AssigneeType = candidate.AssigneeType
					p.AssigneeID = candidate.AssigneeID
					p.intakeRouted = true
				}
			}
		}
	}
	if p.CreatorType != "member" || p.ParentIssueID.Valid || p.AssigneeID.Valid || p.AssigneeType.Valid {
		return nil
	}
	effective := issuestatus.Effective(ctx, q, p.WorkspaceID, p.Status)
	if effective == "done" || effective == "cancelled" {
		return nil
	}
	raw, e := q.GetIssueIntake(ctx, p.WorkspaceID)
	if e != nil {
		return e
	}
	c, e := ParseIssueIntake(raw)
	if e != nil {
		return e
	}
	id := c.SquadFor(util.UUIDToString(projectID))
	if id == "" {
		return nil
	}
	if _, e = q.GetMemberByUserAndWorkspace(ctx, db.GetMemberByUserAndWorkspaceParams{UserID: p.CreatorID, WorkspaceID: p.WorkspaceID}); e != nil {
		return ErrIssueIntakeUnavailable
	}
	squadID, e := util.ParseUUID(id)
	if e != nil {
		return ErrIssueIntakeUnavailable
	}
	if e = ValidateIssueIntakeSquad(ctx, q, p.WorkspaceID, squadID); e != nil {
		return e
	}
	p.intakeRouted = true
	p.AssigneeType = pgtype.Text{String: "squad", Valid: true}
	p.AssigneeID = squadID
	return nil
}
