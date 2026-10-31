package handler

import (
	"context"
	"encoding/json"
	"github.com/multica-ai/multica/server/internal/integrations/ghsnapshot"
	"github.com/multica-ai/multica/server/pkg/workspacerepo"
	"net/http/httptest"
	"testing"
)

func TestCodeDeliverySettings(t *testing.T) {
	if testHandler == nil {
		t.Skip("requires DB")
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), "DELETE FROM repository_configuration WHERE scope='code_delivery' AND subject=$1", testWorkspaceID)
	})
	call := func(body any, want int) workspacerepo.DeliverySettings {
		t.Helper()
		w := httptest.NewRecorder()
		testHandler.CodeDeliverySettings(w, withURLParam(newRequest("PUT", "/code-delivery", body), "id", testWorkspaceID))
		if w.Code != want {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		var c workspacerepo.DeliverySettings
		json.Unmarshal(w.Body.Bytes(), &c)
		return c
	}
	_, _, member := privateAgentTestFixture(t)
	w := httptest.NewRecorder()
	testHandler.CodeDeliverySettings(w, withURLParam(newRequestAs(member, "PUT", "/code-delivery", map[string]any{"revision": 0}), "id", testWorkspaceID))
	if w.Code != 403 {
		t.Fatalf("member changed policy: %d", w.Code)
	}
	c := call(map[string]any{"revision": 0, "default": map[string]string{"mode": "direct", "target_branch": "develop"}}, 200)
	if c.Revision != 1 || c.Default.Mode != "direct" || !c.CanEdit || c.Projects == nil {
		t.Fatal(c)
	}
	call(map[string]any{"revision": 0}, 409)
	call(map[string]any{"revision": 1, "projects": map[string]any{"bad-id": map[string]string{"mode": "direct", "target_branch": "develop"}}}, 400)
	call(map[string]any{"revision": 1, "projects": map[string]any{"00000000-0000-4000-8000-000000000001": map[string]string{"mode": "direct", "target_branch": "develop"}}}, 400)
	c = call(map[string]any{"revision": 1}, 200)
	if c.Default.Mode != "pull_request" || c.Revision != 2 {
		t.Fatal(c)
	}
}
func TestCodeReviewVerdict(t *testing.T) {
	p := workspacerepo.Delivery{AIReviewers: []string{"review-bot"}}
	review := func(id int64, user, sha, state string) ghsnapshot.Review {
		r := ghsnapshot.Review{ID: id, State: state, CommitID: sha, Body: "feedback", SubmittedAt: "2026-10-30T12:00:00Z"}
		r.User.Login = user
		return r
	}
	for _, tc := range []struct {
		name     string
		reviews  []ghsnapshot.Review
		approved bool
		feedback bool
	}{
		{"untrusted", []ghsnapshot.Review{review(1, "stranger", "head", "APPROVED")}, false, false},
		{"old head", []ghsnapshot.Review{review(1, "review-bot", "old", "APPROVED")}, false, false},
		{"approved", []ghsnapshot.Review{review(1, "review-bot", "head", "APPROVED")}, true, false},
		{"changes", []ghsnapshot.Review{review(1, "review-bot", "head", "CHANGES_REQUESTED")}, false, true},
		{"revoked", []ghsnapshot.Review{review(1, "review-bot", "head", "APPROVED"), review(2, "review-bot", "head", "DISMISSED")}, false, false},
		{"fixed", []ghsnapshot.Review{review(1, "review-bot", "head", "CHANGES_REQUESTED"), review(2, "review-bot", "head", "APPROVED")}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, f := reviewVerdict("head", p, tc.reviews, nil)
			if a != tc.approved || (f != "") != tc.feedback {
				t.Fatalf("%v %s", a, f)
			}
		})
	}
	comment := review(3, "review-bot", "head", "")
	comment.UpdatedAt = "2026-10-30T13:00:00Z"
	a, f := reviewVerdict("head", p, []ghsnapshot.Review{review(2, "review-bot", "head", "APPROVED")}, []ghsnapshot.Review{comment})
	if a || f == "" {
		t.Fatal("new feedback ignored")
	}
	_, same := reviewVerdict("head", p, nil, []ghsnapshot.Review{comment})
	if f != same {
		t.Fatal("unstable feedback receipt")
	}
	comment.Body = "edited"
	_, changed := reviewVerdict("head", p, nil, []ghsnapshot.Review{comment})
	if changed == same {
		t.Fatal("comment edit not detected")
	}
}
