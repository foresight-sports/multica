package service

import (
	"context"
	"testing"
)

func TestPlatformFailureNeverDispatchesCoordinator(t *testing.T) {
	for _, reason := range []string{"environment_prepare_failed", "timeout", "runtime_offline", "runtime_recovery", "runtime_cli_timeout", "skill_bundle_unavailable", "invalid_task_identity", "runtime_access_denied", "agent_error.runtime_missing_executable", "agent_error.process_failure"} {
		t.Run(reason, func(t *testing.T) {
			ctx := context.Background()
			f, svc := seedDelegatedFailureFixture(t)
			id := f.insertWorkerTask(t, "failed", "comment", 1, 2)
			// Seed an old recovery signal to exercise replay after a server upgrade.
			target, _, err := svc.ensureDelegatedFailureRecoveryComment(ctx, id)
			if err != nil || target == nil {
				t.Fatalf("seed: %v", err)
			}
			_, err = f.pool.Exec(ctx, `UPDATE agent_task_queue SET failure_reason=$2, error='prepare execution environment: checkout failed' WHERE id=$1`, id, reason)
			if err != nil {
				t.Fatal(err)
			}
			failed, err := svc.Queries.GetAgentTask(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if handled, err := svc.recoverDelegatedTaskFailure(ctx, failed); err != nil || handled {
				t.Fatalf("recovery: handled=%v error=%v", handled, err)
			}
			if err := svc.DispatchDelegatedFailureRecoveryComment(ctx, target.comment, failed.ID); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM agent_task_queue WHERE trigger_evidence_kind='delegated_failure' AND trigger_evidence_ref_id=$1`, id).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("platform failure dispatched %d coordinator tasks", count)
			}
		})
	}
}
