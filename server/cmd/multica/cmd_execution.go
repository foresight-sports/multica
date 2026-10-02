package main

import (
	"context"
	"fmt"
	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/spf13/cobra"
	"os"
)

func init() {
	agentCmd.AddCommand(&cobra.Command{Use: "execution <agent-id>", Short: "List approved execution profiles and selection policy", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newAPIClient(cmd)
		if err != nil {
			return err
		}
		ctx, cancel := cli.APIContext(context.Background())
		defer cancel()
		var policy map[string]any
		if err = client.GetJSON(ctx, "/api/agents/"+args[0]+"/execution", &policy); err != nil {
			return fmt.Errorf("get execution profiles: %w", err)
		}
		return cli.PrintJSON(os.Stdout, policy)
	}})
}
