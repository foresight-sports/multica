package handler

import (
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"testing"
)

func TestProvisionInstanceIntake(t *testing.T) {
	ctx := t.Context()
	sourceWS := dbfx.Workspace(t, "Intake source", "intake-template-source")
	source := dbfx.Agent(t, "Central coordinator", testRuntimeID, testutil.Cols{"workspace_id": sourceWS})
	definition := dbfx.Insert(t, "instance_agent", testutil.Cols{"name": "Intake template test", "created_by": testUserID, "source_agent_id": source})
	if _, err := testPool.Exec(ctx, "UPDATE agent SET instance_agent_id=$1,instance_source_agent_id=id WHERE id=$2", definition, source); err != nil {
		t.Fatal(err)
	}
	template := dbfx.Squad(t, "Ticket Intake", source, testutil.Cols{"workspace_id": sourceWS, "instructions": "Investigate support before proposing changes"})
	if _, err := testPool.Exec(ctx, "UPDATE workspace SET issue_intake=jsonb_build_object('revision',1,'default_squad_id',$1::text) WHERE id=$2", template, sourceWS); err != nil {
		t.Fatal(err)
	}
	target := dbfx.Workspace(t, "Intake target", "intake-template-target")
	leader := dbfx.Agent(t, "Bound coordinator", "", testutil.Cols{"workspace_id": target, "instance_agent_id": definition, "instance_source_agent_id": source})
	specialistDefinition := dbfx.Insert(t, "instance_agent", testutil.Cols{"name": "Intake specialist test", "created_by": testUserID})
	dbfx.Agent(t, "Specialist", "", testutil.Cols{"workspace_id": target, "instance_agent_id": specialistDefinition})
	dbfx.Agent(t, "Local only", "", testutil.Cols{"workspace_id": target})
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	for range 2 {
		if err := provisionInstanceIntake(ctx, q, parseUUID(target), parseUUID(testUserID)); err != nil {
			t.Fatal(err)
		}
	}
	squads, err := q.ListSquads(ctx, parseUUID(target))
	if err != nil || len(squads) != 1 {
		t.Fatalf("expected exactly one squad: %+v %v", squads, err)
	}
	squad := squads[0]
	if uuidToString(squad.LeaderID) != leader || squad.Instructions != "Investigate support before proposing changes" {
		t.Fatalf("wrong template binding: %+v", squad)
	}
	members, err := q.ListSquadMembers(ctx, squad.ID)
	if err != nil || len(members) != 3 {
		t.Fatalf("expected all workspace agents: %+v %v", members, err)
	}
	raw, err := q.GetIssueIntake(ctx, parseUUID(target))
	if err != nil {
		t.Fatal(err)
	}
	config, err := service.ParseIssueIntake(raw)
	if err != nil || config.DefaultSquadID != uuidToString(squad.ID) || config.Revision != 1 {
		t.Fatalf("invalid intake: %+v %v", config, err)
	}

	foundLeader := false
	for _, m := range members {
		if uuidToString(m.MemberID) == leader {
			foundLeader = m.Role == "leader"
		}
	}
	if !foundLeader {
		t.Fatal("missing leader role")
	}
	// An archived coordinator must not create another squad.
	if _, err := tx.Exec(ctx, "SAVEPOINT guards"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "UPDATE workspace SET issue_intake='{}' WHERE id=$1", target); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "UPDATE agent SET archived_at=now() WHERE id=$1", leader); err != nil {
		t.Fatal(err)
	}
	if err := provisionInstanceIntake(ctx, q, parseUUID(target), parseUUID(testUserID)); err != nil {
		t.Fatal(err)
	}
	unchanged, _ := q.ListSquads(ctx, parseUUID(target))
	if len(unchanged) != 1 {
		t.Fatal("created squad for archived coordinator")
	}
	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT guards"); err != nil {
		t.Fatal(err)
	}
	// Existing opt-out configuration is respected, even without a squad.
	if _, err := tx.Exec(ctx, "UPDATE workspace SET issue_intake='{\"revision\":2}' WHERE id=$1", target); err != nil {
		t.Fatal(err)
	}
	if err := provisionInstanceIntake(ctx, q, parseUUID(target), parseUUID(testUserID)); err != nil {
		t.Fatal(err)
	}
	raw, _ = q.GetIssueIntake(ctx, parseUUID(target))
	config, _ = service.ParseIssueIntake(raw)
	if config.DefaultSquadID != "" || config.Revision != 2 {
		t.Fatal("overwrote an explicit opt-out")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	rolledBack, err := testHandler.Queries.ListSquads(ctx, parseUUID(target))
	if err != nil || len(rolledBack) != 0 {
		t.Fatalf("rollback left squad records: %v %v", rolledBack, err)
	}

}
