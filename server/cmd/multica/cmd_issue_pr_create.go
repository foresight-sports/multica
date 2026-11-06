package main

import (
	"context"
	"fmt"
	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
	"strings"
)

func init() { issueCmd.AddCommand(newIssuePRCreateCmd()) }

func newIssuePRCreateCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "pr-create <issue-id>", Short: "Check current Jira policy and open a normal GitHub pull request", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		title, _ := cmd.Flags().GetString("title")
		body, _ := cmd.Flags().GetString("body-file")
		base, _ := cmd.Flags().GetString("base")
		head, _ := cmd.Flags().GetString("head")
		if strings.TrimSpace(title) == "" || body == "" || base == "" {
			return fmt.Errorf("--title, --body-file and --base are required")
		}
		if _, err := os.Stat(body); err != nil {
			return fmt.Errorf("read PR body: %w", err)
		}
		client, err := newAPIClient(cmd)
		if err != nil {
			return err
		}
		ctx, cancel := cli.APIContext(context.Background())
		defer cancel()
		issue, err := resolveIssueRef(ctx, client, args[0])
		if err != nil {
			return err
		}
		var policy struct {
			Allowed bool   `json:"allowed"`
			Key     string `json:"jira_ticket_id"`
		}
		if err = client.GetJSON(ctx, "/api/issues/"+issue.ID+"/pull-request-check", &policy); err != nil {
			return err
		}
		if !policy.Allowed {
			return fmt.Errorf("PR policy did not authorize creation")
		}
		if policy.Key != "" && !strings.Contains(title, policy.Key) {
			title = policy.Key + ": " + title
		}
		argv := []string{"pr", "create", "--title", title, "--body-file", body, "--base", base}
		if head != "" {
			argv = append(argv, "--head", head)
		}
		child := exec.CommandContext(cmd.Context(), "gh", argv...)
		child.Stdin = os.Stdin
		child.Stdout = cmd.OutOrStdout()
		child.Stderr = cmd.ErrOrStderr()
		if err = child.Run(); err != nil {
			return fmt.Errorf("GitHub PR creation failed: %w", err)
		}
		return nil
	}}
	for _, f := range []string{"title", "body-file", "base", "head"} {
		cmd.Flags().String(f, "", f+" for the pull request")
	}
	return cmd
}
