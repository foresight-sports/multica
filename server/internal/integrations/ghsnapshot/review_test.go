package ghsnapshot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestReviewRequestsRevalidateAndDeduplicate(t *testing.T) {
	state, head := "APPROVED", "head"
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/org/repo/pulls/1":
			json.NewEncoder(w).Encode(map[string]any{"state": "open", "head": map[string]string{"sha": head}, "user": map[string]string{"login": "author"}, "requested_reviewers": []map[string]string{{"login": "already"}}})
		case "/repos/org/repo/pulls/1/reviews":
			json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "state": state, "commit_id": "head", "user": map[string]string{"login": "bot"}}, {"id": 2, "state": "COMMENTED", "commit_id": "head", "user": map[string]string{"login": "reviewed"}}})
		case "/repos/org/repo/pulls/1/requested_reviewers":
			posts++
			var body struct {
				Reviewers []string `json:"reviewers"`
				Teams     []string `json:"team_reviewers"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if len(body.Reviewers) != 1 || body.Reviewers[0] != "human" || len(body.Teams) != 1 {
				t.Errorf("unexpected request %#v", body)
			}
			w.WriteHeader(201)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := newTestClient(t, srv.URL)
	c.tokens[1] = cachedToken{token: "test", expiry: time.Now().Add(time.Hour)}
	request := func() error {
		return c.RequestReviewers(t.Context(), 1, "org", "repo", 1, "head", []string{"human", "author", "already", "reviewed"}, []string{"team"}, []string{"bot"})
	}
	if e := request(); e != nil {
		t.Fatal(e)
	}
	head = "new"
	if request() == nil {
		t.Fatal("stale head accepted")
	}
	head = "head"
	state = "DISMISSED"
	if request() == nil {
		t.Fatal("dismissed approval accepted")
	}
	if posts != 1 {
		t.Fatalf("posts %d", posts)
	}
}
func TestReviewPaginationFailsClosed(t *testing.T) {
	pages := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { pages++; json.NewEncoder(w).Encode(make([]Review, 100)) }))
	defer srv.Close()
	c := newTestClient(t, srv.URL)
	c.tokens[1] = cachedToken{token: "test", expiry: time.Now().Add(time.Hour)}
	if _, e := c.Reviews(t.Context(), 1, "org", "repo", 1, false); e == nil {
		t.Fatal("truncated approvals accepted")
	}
	if pages != 10 {
		t.Fatal(pages)
	}
}
