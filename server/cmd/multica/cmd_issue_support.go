package main

import (
	"context"
	"fmt"
	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

func init() {
	for _, kind := range []string{"context", "reply", "file"} {
		cmd := &cobra.Command{Use: "support-" + kind + " <issue-id>", Short: "Read Slack support evidence or submit a coordinator-reviewed CS response", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			client, e := newAPIClient(cmd)
			if e != nil {
				return e
			}
			ctx, cancel := context.WithTimeout(context.Background(), cli.AtLeastAPITimeout(60*time.Second))
			defer cancel()
			issue, e := resolveIssueRef(ctx, client, args[0])
			if e != nil {
				return e
			}
			var out any
			switch kind {
			case "context":
				e = client.GetJSON(ctx, "/api/issues/"+issue.ID+"/support-context", &out)
			case "reply":
				body, _, err := resolveTextFlag(cmd, "body")
				if err != nil {
					return err
				}
				evidence, _, err := resolveTextFlag(cmd, "evidence")
				if err != nil {
					return err
				}
				revision, _ := cmd.Flags().GetInt64("revision")
				confidence, _ := cmd.Flags().GetFloat64("confidence")
				outcome, _ := cmd.Flags().GetString("outcome")
				if !cmd.Flags().Changed("confidence") {
					return fmt.Errorf("--confidence is required")
				}
				e = client.PostJSON(ctx, "/api/issues/"+issue.ID+"/support-reply", map[string]any{"revision": revision, "confidence": confidence, "outcome": outcome, "body": body, "evidence": evidence}, &out)
			case "file":
				id, _ := cmd.Flags().GetString("file-id")
				dest, _ := cmd.Flags().GetString("output")
				if !regexp.MustCompile(`^F[A-Z0-9]+$`).MatchString(id) || dest == "" {
					return fmt.Errorf("--file-id and --output are required")
				}
				data, err := client.DownloadFile(ctx, "/api/issues/"+issue.ID+"/support-files/"+id)
				if err != nil {
					return err
				}
				// Never overwrite an existing file or follow an existing symlink.
				f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
				if err != nil {
					return err
				}
				_, err = f.Write(data)
				closeErr := f.Close()
				if err != nil {
					return err
				}
				if closeErr != nil {
					return closeErr
				}
				abs, _ := filepath.Abs(dest)
				out = map[string]any{"path": abs, "bytes": len(data)}
			}
			if e != nil {
				return e
			}
			return cli.PrintJSON(cmd.OutOrStdout(), out)
		}}
		if kind == "reply" {
			for _, flag := range []string{"body", "evidence"} {
				cmd.Flags().String(flag, "", flag)
				cmd.Flags().String(flag+"-file", "", "Read "+flag+" from file")
				cmd.Flags().Bool(flag+"-stdin", false, "Read "+flag+" from stdin")
			}
			cmd.Flags().Int64("revision", 0, "Conversation revision from support-context")
			cmd.Flags().Float64("confidence", 0, "Evidence-based confidence from 0 to 1")
			cmd.Flags().String("outcome", "", "answer, clarify, defect or escalate")
		}
		if kind == "file" {
			cmd.Flags().String("file-id", "", "Slack file ID from support-context")
			cmd.Flags().String("output", "", "New local file path")
		}
		issueCmd.AddCommand(cmd)
	}
}
