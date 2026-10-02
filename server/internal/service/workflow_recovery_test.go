package service

import "testing"

func TestCoordinatorCallbackFailureDoesNotWakeWorker(t *testing.T) {
	f, svc := seedDelegatedFailureFixture(t)
	failedID := f.insertWorkerTask(t, "failed", "comment", 1, 2)
	// The backward attribution edge of a coordinator callback points to the
	// worker's reply. That worker is not the owner of coordinator recovery.
	if _, err := f.pool.Exec(t.Context(), "UPDATE agent_task_queue SET is_leader_task=true WHERE id=$1", failedID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), "UPDATE agent_task_queue SET is_leader_task=false WHERE id=$1", f.sourceTask); err != nil {
		t.Fatal(err)
	}
	failed, err := svc.Queries.GetAgentTask(t.Context(), failedID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		handled, err := svc.recoverDelegatedTaskFailure(t.Context(), failed)
		if err != nil || handled {
			t.Fatalf("callback failure delegated backward: %v %v", handled, err)
		}
	}
}
