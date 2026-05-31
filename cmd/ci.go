package cmd

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/tui"
	"github.com/spf13/cobra"
)

func ciCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ci",
		Short: "Set up CI integration for bark",
		Run: func(cmd *cobra.Command, args []string) {
			if err := runCIcmd(cmd); err != nil {
				PrintError(err)
			}
		},
	}

	return cmd
}

func runCIcmd(cmd *cobra.Command) error {
	cfg := config.New()

	m := tui.New(tui.Options{
		Task:   tui.TaskCI,
		Config: cfg,
	})

	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running UI: %w", err)
	}

	return nil
}
