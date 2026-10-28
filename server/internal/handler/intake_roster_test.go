package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestIntakeRosterAutomaticallyIncludesWorkspaceAgents(t *testing.T) {
	ctx := context.Background()
	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	var ws, squad, leader, other string
	if err := tx.QueryRow(ctx, `INSERT INTO workspace (name, slug) VALUES ('intake roster test', gen_random_uuid()::text) RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO agent (workspace_id, name, runtime_mode, owner_id) VALUES ($1, 'Coordinator', 'local', $2) RETURNING id`, ws, testUserID).Scan(&leader); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO squad (workspace_id, name, leader_id, creator_id) VALUES ($1, 'Intake', $2, $3) RETURNING id`, ws, leader, testUserID).Scan(&squad); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO squad (workspace_id, name, leader_id, creator_id) VALUES ($1, 'Manual squad', $2, $3) RETURNING id`, ws, leader, testUserID).Scan(&other); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE workspace SET issue_intake = jsonb_build_object('default_squad_id', $2::text) WHERE id=$1`, ws, squad)
	exec(`INSERT INTO agent (workspace_id, name, description, runtime_mode, owner_id) VALUES ($1, 'Jonny JIRA', 'Jira operations across projects', 'local', $2)`, ws, testUserID)
	exec(`INSERT INTO agent (workspace_id, name, kind, runtime_mode, owner_id) VALUES ($1, 'Hidden builder', 'system', 'local', $2)`, ws, testUserID)
	exec(`INSERT INTO agent (workspace_id, name, archived_at, runtime_mode, owner_id) VALUES ($1, 'Retired', now(), 'local', $2)`, ws, testUserID)
	exec(`INSERT INTO agent (workspace_id, name, runtime_mode, owner_id) VALUES ($1, 'Other workspace specialist', 'local', $2)`, testWorkspaceID, testUserID)
	checkCount := func(id string, want int) {
		t.Helper()
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM squad_member WHERE squad_id=$1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Fatalf("members = %d, want %d", n, want)
		}
	}
	checkCount(squad, 2)
	checkCount(other, 0)
	exec(`UPDATE squad_member SET role='Preserved role' WHERE squad_id=$1 AND member_id=$2`, squad, leader)
	exec(`SELECT sync_intake_roster($1)`, ws)
	checkCount(squad, 2)
	var role string
	if err := tx.QueryRow(ctx, `SELECT role FROM squad_member WHERE squad_id=$1 AND member_id=$2`, squad, leader).Scan(&role); err != nil || role != "Preserved role" {
		t.Fatalf("role changed: %q, %v", role, err)
	}
	exec(`UPDATE workspace SET issue_intake = jsonb_build_object('projects', jsonb_build_object('project', $2::text)) WHERE id=$1`, ws, other)
	checkCount(other, 2)
	q := db.New(tx)
	s, err := q.GetSquad(ctx, util.MustParseUUID(squad))
	if err != nil {
		t.Fatal(err)
	}
	roster := buildSquadRoster(ctx, q, s)
	if !strings.Contains(roster, "Jira operations across projects") || strings.Contains(roster, "Hidden builder") || strings.Contains(roster, "Retired") || strings.Contains(roster, "Other workspace specialist") {
		t.Fatalf("unexpected roster: %s", roster)
	}
	exec(`UPDATE agent SET archived_at=NULL WHERE workspace_id=$1 AND name='Retired'`, ws)
	checkCount(other, 3)
}
