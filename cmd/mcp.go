package cmd

import (
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/ionut-t/bark/v2/internal/mcpserver"
	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/ionut-t/bark/v2/internal/version"
	"github.com/spf13/cobra"
)

func mcpCmd(getCfg configFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve Bark's tools to agents over MCP (stdio)",
		Long: `Run Bark as a Model Context Protocol server on stdin and stdout, for agents
such as Claude Code, Cursor or Zed. Start it from the repository to review;
the agent's MCP client launches it, for example:

  claude mcp add bark -- bark mcp

Tools:
  review             Bark's review of a change as JSON findings, after Bark's
                     check if it's on; calls the configured model (costs
                     money), posts nothing
  review_diff        the change as Bark's reviewer sees it: .barkignore and
                     .gitignore applied, commits, diffstat and related code
                     (no model call)
  earlier_findings   what Bark's earlier reviews of a pull request raised
                     (no model call)`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := getCfg()
			dirs, err := config.Paths()
			if err != nil {
				return err
			}
			// Listed in the review tool's description; without them the
			// agent can still name a reviewer file.
			var names []string
			if list, err := reviewers.Get(dirs.Config); err == nil {
				for _, r := range list {
					names = append(names, r.Name)
				}
			}

			return mcpserver.Run(cmd.Context(), mcpserver.Options{
				Version:   version.Version(),
				Limit:     git.LimitFrom(cfg),
				Config:    cfg,
				Paths:     dirs,
				Reviewers: names,
			})
		},
	}
}
