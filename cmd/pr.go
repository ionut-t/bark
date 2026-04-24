package cmd

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/pkg/plain"
	"github.com/ionut-t/bark/v2/tui"
	"github.com/spf13/cobra"
)

func prCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr",
		Short: "Generate a pull request message",
		Run: func(cmd *cobra.Command, args []string) {
			if err := runPRCmd(cmd); err != nil {
				if hasStdinData() || isPlainMode(cmd) {
					plain.Errf("%s", err)
				} else {
					PrintError(err)
				}
			}
		},
	}

	cmd.Flags().StringP("branch", "b", "", "The base branch to compare against (optional)")
	cmd.Flags().StringP("pr", "p", "", "Generate a description for a GitHub pull request by number (requires gh CLI)")

	cmd.MarkFlagsMutuallyExclusive("branch", "pr")

	return cmd
}

func runPRCmd(cmd *cobra.Command) error {
	branch, _ := cmd.Flags().GetString("branch")
	pr, _ := cmd.Flags().GetString("pr")

	cfg := config.New()

	stdinDiff, err := readStdinIfPiped()
	if err != nil {
		return err
	}

	if stdinDiff != nil || isPlainMode(cmd) {
		return plain.RunPR(plain.PROptions{
			Diff:   stdinDiff,
			Branch: branch,
			PR:     pr,
			Config: cfg,
		})
	}

	// TUI mode
	storage, err := config.GetStorage()
	if err != nil {
		return fmt.Errorf("error getting storage: %w", err)
	}

	m := tui.New(tui.Options{
		Task:    tui.TaskPRDescription,
		Storage: storage,
		Config:  cfg,
		Branch:  branch,
		PR:      pr,
	})

	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running UI: %w", err)
	}

	return nil
}
