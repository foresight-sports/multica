package service

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

func TestQuickCreateReceiptLifecycle(t *testing.T) {
	for _, outcome := range []string{"completed", "failed_retry", "cancelled"} {
		t.Run(outcome, func(t *testing.T) {
			pool := newResolveOriginatorPool(t)
			ctx := context.Background()
			workspaceID, userID, agentID, issueID := seedAttributionFixture(t, pool)
			q := db.New(pool)
			svc := &TaskService{Queries: q, TxStarter: pool, Bus: events.New()}
			task, err := svc.EnqueueQuickCreateTask(ctx, util.MustParseUUID(workspaceID), util.MustParseUUID(userID), util.MustParseUUID(agentID), pgtype.UUID{}, "Investigate device entitlements", "medium", "", pgtype.UUID{}, pgtype.UUID{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				pool.Exec(context.Background(), `DELETE FROM inbox_item WHERE workspace_id=$1`, workspaceID)
				pool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE agent_id=$1`, agentID)
			})
			qc, ok := svc.parseQuickCreateContext(task)
			if !ok || qc.InboxItemID != util.UUIDToString(task.ID) {
				t.Fatal("missing durable receipt identity")
			}
			var kind, prompt string
			if err := pool.QueryRow(ctx, `SELECT type, details->>'original_prompt' FROM inbox_item WHERE id=$1 AND recipient_id=$2 AND workspace_id=$3`, task.ID, userID, workspaceID).Scan(&kind, &prompt); err != nil {
				t.Fatal(err)
			}
			if kind != "quick_create_pending" || prompt != qc.Prompt {
				t.Fatalf("receipt = %s/%s", kind, prompt)
			}
			if _, err := pool.Exec(ctx, `UPDATE inbox_item SET read=true, archived=true WHERE id=$1`, task.ID); err != nil {
				t.Fatal(err)
			}
			want := "quick_create_done"
			switch outcome {
			case "completed":
				if _, err := pool.Exec(ctx, `UPDATE issue SET origin_type='quick_create',origin_id=$1 WHERE id=$2`, task.ID, issueID); err != nil {
					t.Fatal(err)
				}
				svc.notifyQuickCreateCompleted(ctx, task, qc, nil)
			case "failed_retry":
				// A retry keeps the request context but has a different run ID.
				task.ID = dbid.NewV7()
				svc.notifyQuickCreateFailed(ctx, task, qc, "runtime unavailable")
				want = "quick_create_failed"
			case "cancelled":
				task.Status = "cancelled"
				svc.NotifyTaskFinished(task)
				want = "quick_create_unconfirmed"
			}
			var count int
			var read, archived bool
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM inbox_item WHERE workspace_id=$1`, workspaceID).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `SELECT type,read,archived FROM inbox_item WHERE id=$1`, qc.InboxItemID).Scan(&kind, &read, &archived); err != nil {
				t.Fatal(err)
			}
			if count != 1 || kind != want || read || archived {
				t.Fatalf("receipt count=%d type=%s read=%v archived=%v", count, kind, read, archived)
			}
		})
	}
}
