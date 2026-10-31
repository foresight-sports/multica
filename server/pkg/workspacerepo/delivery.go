package workspacerepo

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/multica-ai/multica/server/pkg/workflow"
)

// Delivery is administered by humans; project overrides replace the whole policy.
// Empty mode means the safe, backwards-compatible pull request default.
type Delivery struct {
	Mode           string   `json:"mode"`
	TargetBranch   string   `json:"target_branch"`
	AIReviewers    []string `json:"ai_reviewers"`
	HumanReviewers []string `json:"human_reviewers"`
	HumanTeams     []string `json:"human_teams"`
}

type DeliverySettings struct {
	AutomationAvailable bool                `json:"automation_available"`
	AutomationErrors    []string            `json:"automation_errors"`
	Default             Delivery            `json:"default"`
	Projects            map[string]Delivery `json:"projects"`
	Revision            int64               `json:"revision"`
	CanEdit             bool                `json:"can_edit"`
}

var githubLogin = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*(\[bot\])?$`)
var githubTeam = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

func (p *Delivery) Validate() error {
	if p.Mode == "" {
		p.Mode = "pull_request"
	}
	if p.Mode != "pull_request" && p.Mode != "direct" {
		return fmt.Errorf("choose pull_request or direct delivery")
	}
	p.TargetBranch = strings.TrimSpace(p.TargetBranch)
	if (p.Mode == "direct" && p.TargetBranch == "") || (p.TargetBranch != "" && !workflow.ValidBranch(p.TargetBranch)) {
		return fmt.Errorf("direct delivery requires a valid target branch")
	}
	for i, list := range []*[]string{&p.AIReviewers, &p.HumanReviewers, &p.HumanTeams} {
		if len(*list) > 15 {
			return fmt.Errorf("at most 15 reviewers or teams per list")
		}
		seen := map[string]bool{}
		normalized := []string{}
		for _, name := range *list {
			name = strings.ToLower(strings.TrimSpace(name))
			if name == "" {
				continue
			}
			re := githubLogin
			if i == 2 {
				re = githubTeam
			}
			if len(name) > 100 || !re.MatchString(name) {
				return fmt.Errorf("invalid GitHub reviewer or team slug")
			}
			if !seen[name] {
				normalized = append(normalized, name)
				seen[name] = true
			}
		}
		*list = normalized
	}
	for _, ai := range p.AIReviewers {
		for _, human := range p.HumanReviewers {
			if ai == human {
				return fmt.Errorf("AI and human reviewers must be distinct")
			}
		}
	}
	if len(p.AIReviewers) == 0 && (len(p.HumanReviewers) > 0 || len(p.HumanTeams) > 0) {
		return fmt.Errorf("configure an AI reviewer identity before enabling human review handoff")
	}
	return nil
}

func (c DeliverySettings) Effective(project string) Delivery {
	if p, ok := c.Projects[project]; ok {
		return p
	}
	return c.Default
}

func (p Delivery) Instructions(repository, base string) string {
	if p.TargetBranch == "" {
		p.TargetBranch = base
	}
	raw, _ := json.Marshal(p)
	common := "\n\n## Code delivery policy (current administrator configuration)\nRepository: " + repository + "\n" + string(raw) + "\nUse this current policy instead of historical branch names or draft-PR advice in stored context. Work in your isolated task branch and preserve branch leases and pushed checkpoints. Never force-push or bypass repository protection.\n"
	if p.Mode == "direct" {
		return common + "After review and passing checks, fetch the target branch, integrate its latest commits into the task branch, and validate the resulting commit. Push that exact commit to the configured target using a normal fast-forward push. If the target advances, fetch, reconcile and revalidate before retrying. Do not reset another checkout or overwrite work. No PR is required. Record the delivered SHA and verification on the issue; only mark Done once all requested work is delivered. Deployment still follows its separate approval policy and creates a new versioned release branch.\n"
	}
	return common + "Open a normal, ready-for-review PR, never a draft by default. Link it to this Multica issue. Address actionable AI review comments and requested changes, push fixes, and obtain a fresh review of the new head. Do not treat stale approval as approval of new commits. Configured GitHub AI reviewer identities control automatic feedback wakeups and human review handoff; if none are configured, report that automation is not configured. Keep the issue in review until all linked PRs are merged; merge events complete it according to workspace merge settings. Human review requests do not authorize merging.\n"
}
