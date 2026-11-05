package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Called under the instance provisioning advisory lock, in the workspace-create
// transaction. Do not replace an administrator's existing intake configuration.
func provisionInstanceIntake(ctx context.Context, q *db.Queries, workspaceID, creatorID pgtype.UUID) error {
	raw, err := q.GetIssueIntake(ctx, workspaceID)
	if err != nil {
		return err
	}
	config, err := service.ParseIssueIntake(raw)
	if err != nil {
		return err
	}
	if config.Revision != 0 || config.DefaultSquadID != "" || len(config.Projects) != 0 {
		return nil
	}
	templates, err := q.ListInstanceIntakeTemplates(ctx)
	if err != nil {
		return err
	}
	if len(templates) == 0 {
		return nil
	}
	if len(templates) != 1 {
		return fmt.Errorf("multiple central intake squads are configured")
	}
	template := templates[0]
	source, err := q.GetAgent(ctx, template.LeaderID)
	if err != nil {
		return err
	}
	agents, err := q.ListAgents(ctx, workspaceID)
	if err != nil {
		return err
	}
	var leader pgtype.UUID
	for _, agent := range agents {
		if agent.InstanceAgentID.Valid && agent.InstanceAgentID == source.InstanceAgentID {
			leader = agent.ID
		}
	}
	// An archived/opted-out coordinator must never be silently re-enabled.
	if !leader.Valid {
		return nil
	}
	squad, err := q.CreateSquad(ctx, db.CreateSquadParams{WorkspaceID: workspaceID, Name: template.Name,
		Description: template.Description, LeaderID: leader, CreatorID: creatorID, AvatarUrl: template.AvatarUrl})
	if err != nil {
		return err
	}
	if _, err = q.UpdateSquad(ctx, db.UpdateSquadParams{ID: squad.ID, Instructions: pgtype.Text{String: template.Instructions, Valid: true}}); err != nil {
		return err
	}

	templateMembers, err := q.ListSquadMembers(ctx, template.ID)
	if err != nil {
		return err
	}
	roles := map[pgtype.UUID]string{}
	for _, member := range templateMembers {
		if member.MemberType != "agent" {
			continue
		}
		agent, err := q.GetAgent(ctx, member.MemberID)
		if err != nil {
			return err
		}
		if agent.WorkspaceID == template.WorkspaceID && agent.InstanceAgentID.Valid {
			roles[agent.InstanceAgentID] = member.Role
		}
	}
	// The intake coordinator can consider every available instance agent.
	// Membership does not change invocation permissions or runtime grants.
	for _, agent := range agents {
		if !agent.InstanceAgentID.Valid {
			continue
		}
		role := roles[agent.InstanceAgentID]
		if agent.ID == leader {
			role = "leader"
		}
		if _, err = q.AddSquadMember(ctx, db.AddSquadMemberParams{SquadID: squad.ID, MemberType: "agent", MemberID: agent.ID, Role: role}); err != nil {
			return err
		}
	}
	config.Revision = 1
	config.DefaultSquadID = uuidToString(squad.ID)
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	_, err = q.UpdateIssueIntake(ctx, db.UpdateIssueIntakeParams{WorkspaceID: workspaceID, Revision: 0, Config: data})
	return err
}
