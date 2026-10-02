package daemon

import (
	"strings"
	"testing"
)

func TestJiraTaskTracking(t *testing.T) {
	task := Task{IssueID: "issue-id", IssueIdentifier: "APP-3", JiraTicketID: "FS-123"}
	prompt := BuildPrompt(task, "codex")
	for _, want := range []string{"JIRA ticket ID: FS-123", "pull request", "changelog", "Multica issue IDs"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("missing %s", want)
		}
	}
	if taskBranchIdentifier(task) != "FS-123" {
		t.Fatal("branch did not use JIRA key")
	}
	task.JiraTicketID = ""
	if taskBranchIdentifier(task) != "APP-3" {
		t.Fatal("legacy branch identity changed")
	}
}
