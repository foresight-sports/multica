package ghsnapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Review struct {
	ID          int64  `json:"id"`
	State       string `json:"state"`
	CommitID    string `json:"commit_id"`
	Body        string `json:"body"`
	UpdatedAt   string `json:"updated_at"`
	SubmittedAt string `json:"submitted_at"`
	User        struct {
		Login string `json:"login"`
	} `json:"user"`
}
type ReviewPull struct {
	State string `json:"state"`
	Draft bool   `json:"draft"`
	Head  struct {
		SHA string `json:"sha"`
	} `json:"head"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	RequestedReviewers []struct {
		Login string `json:"login"`
	} `json:"requested_reviewers"`
	RequestedTeams []struct {
		Slug string `json:"slug"`
	} `json:"requested_teams"`
}

func reviewPath(owner, repo string, number int32) string {
	return fmt.Sprintf("/repos/%s/%s/pulls/%d", url.PathEscape(owner), url.PathEscape(repo), number)
}
func (c *Client) reviewREST(ctx context.Context, installation int64, method, path string, body any, out any) error {
	if !c.Enabled() {
		return fmt.Errorf("GitHub App is not configured")
	}
	token, err := c.installationToken(ctx, installation)
	if err != nil {
		return err
	}
	var b []byte
	if body != nil {
		b, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.apiBase+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GitHub review request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 403 || resp.StatusCode == 429 {
		return rateLimitFromResponse(resp, c.now())
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GitHub review API status %d", resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
	}
	return nil
}
func (c *Client) ReviewPull(ctx context.Context, installation int64, owner, repo string, number int32) (ReviewPull, error) {
	var p ReviewPull
	e := c.reviewREST(ctx, installation, "GET", reviewPath(owner, repo, number), nil, &p)
	return p, e
}
func (c *Client) Reviews(ctx context.Context, installation int64, owner, repo string, number int32, comments bool) ([]Review, error) {
	suffix := "reviews"
	if comments {
		suffix = "comments"
	}
	all := []Review{}
	for page := 1; page <= 10; page++ {
		var batch []Review
		if e := c.reviewREST(ctx, installation, "GET", fmt.Sprintf("%s/%s?per_page=100&page=%d", reviewPath(owner, repo, number), suffix, page), nil, &batch); e != nil {
			return nil, e
		}
		all = append(all, batch...)
		if len(batch) < 100 {
			return all, nil
		}
	}
	return nil, fmt.Errorf("review history exceeds automation limit; manual review required")
}

// RequestReviewers rechecks the live head and skips existing requests and reviews.
// GitHub has no conditional request-reviewers API; this never merges a PR.
func (c *Client) RequestReviewers(ctx context.Context, installation int64, owner, repo string, number int32, head string, users, teams, ai []string) error {
	p, e := c.ReviewPull(ctx, installation, owner, repo, number)
	if e != nil {
		return e
	}
	if p.Head.SHA != head || p.State != "open" || p.Draft {
		return fmt.Errorf("pull request changed before reviewer handoff")
	}
	existing := map[string]bool{strings.ToLower(p.User.Login): true}
	existingTeams := map[string]bool{}
	for _, u := range p.RequestedReviewers {
		existing[strings.ToLower(u.Login)] = true
	}
	for _, t := range p.RequestedTeams {
		existingTeams[strings.ToLower(t.Slug)] = true
	}
	reviews, e := c.Reviews(ctx, installation, owner, repo, number, false)
	if e != nil {
		return e
	}
	latestAI := map[string]Review{}
	for _, r := range reviews {
		if r.CommitID == head && r.State != "PENDING" {
			login := strings.ToLower(r.User.Login)
			if old, ok := latestAI[login]; !ok || r.ID > old.ID {
				latestAI[login] = r
			}
		}
	}
	for _, login := range ai {
		if latestAI[strings.ToLower(login)].State != "APPROVED" {
			return fmt.Errorf("AI approval changed before human review handoff")
		}
	}
	for _, r := range reviews {
		if r.CommitID == head && r.State != "PENDING" {
			existing[strings.ToLower(r.User.Login)] = true
		}
	}
	pendingUsers := []string{}
	pendingTeams := []string{}
	for _, u := range users {
		if !existing[strings.ToLower(u)] {
			pendingUsers = append(pendingUsers, u)
		}
	}
	for _, t := range teams {
		if !existingTeams[strings.ToLower(t)] {
			pendingTeams = append(pendingTeams, t)
		}
	}
	if len(pendingUsers)+len(pendingTeams) == 0 {
		return nil
	}
	return c.reviewREST(ctx, installation, "POST", reviewPath(owner, repo, number)+"/requested_reviewers", map[string]any{"reviewers": pendingUsers, "team_reviewers": pendingTeams}, nil)
}
