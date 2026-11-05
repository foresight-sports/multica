package main

import (
	"context"
	"fmt"
	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/spf13/cobra"
	"os"
)

func init() {
	workspaceCmd.AddCommand(&cobra.Command{Use: "links", Short: "List linked workspaces available to the original requester", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := newAPIClient(cmd)
		if err != nil {
			return err
		}
		ctx, cancel := cli.APIContext(context.Background())
		defer cancel()
		var result any
		if err = client.GetJSON(ctx, "/api/workspaces/"+client.WorkspaceID+"/workspace-links", &result); err != nil {
			return err
		}
		return cli.PrintJSON(os.Stdout, result)
	}})
	for _, create := range []bool{false, true} {
		name := "linked-requests"
		help := "Read scoped SDK requests and their latest results"
		if create {
			name = "linked-request"
			help = "Send a scoped request through a linked workspace's Intake Coordinator"
		}
		cmd := &cobra.Command{Use: name + " <issue-id>", Short: help, Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
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
			path := "/api/issues/" + issue.ID + "/workspace-requests"
			var result any
			if create {
				ws, _ := cmd.Flags().GetString("target-workspace")
				key, _ := cmd.Flags().GetString("request-key")
				title, _ := cmd.Flags().GetString("title")
				description, _, err := resolveTextFlag(cmd, "description")
				if err != nil {
					return err
				}
				if ws == "" || key == "" || title == "" {
					return fmt.Errorf("--target-workspace, --request-key and --title are required; reuse the same request key when retrying")
				}
				err = client.PostJSON(ctx, path, map[string]string{"workspace_id": ws, "request_key": key, "title": title, "description": description}, &result)
				if err != nil {
					return err
				}
			} else if err = client.GetJSON(ctx, path, &result); err != nil {
				return err
			}
			return cli.PrintJSON(os.Stdout, result)
		}}
		if create {
			for _, flag := range []string{"target-workspace", "request-key", "title", "description", "description-file"} {
				cmd.Flags().String(flag, "", flag+" for this linked request")
			}
			cmd.Flags().Bool("description-stdin", false, "Read description from stdin")
		}
		issueCmd.AddCommand(cmd)
	}
}
