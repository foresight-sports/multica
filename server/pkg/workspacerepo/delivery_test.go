package workspacerepo

import (
	"strings"
	"testing"
)

func TestDeliveryValidation(t *testing.T) {
	for _, p := range []Delivery{{Mode: "draft"}, {Mode: "direct"}, {Mode: "direct", TargetBranch: "../oops"}, {AIReviewers: []string{"bot"}, HumanReviewers: []string{"BOT"}}, {HumanReviewers: []string{"person"}}, {AIReviewers: []string{"../../repo"}}} {
		if p.Validate() == nil {
			t.Fatalf("accepted %#v", p)
		}
	}
	p := Delivery{AIReviewers: []string{"Bot[bot]", "bot[bot]", ""}}
	if e := p.Validate(); e != nil {
		t.Fatal(e)
	}
	if p.Mode != "pull_request" || len(p.AIReviewers) != 1 {
		t.Fatal(p)
	}
	c := DeliverySettings{Default: p, Projects: map[string]Delivery{"project": {Mode: "direct", TargetBranch: "develop"}}}
	if c.Effective("other").Mode != "pull_request" || c.Effective("project").Mode != "direct" {
		t.Fatal(c)
	}
	if !strings.Contains(c.Effective("project").Instructions("org/repo", "old-release"), "normal fast-forward push") {
		t.Fatal("missing direct safety policy")
	}
}
