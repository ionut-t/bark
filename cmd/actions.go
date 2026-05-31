package cmd

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/tui"
	"github.com/spf13/cobra"
)

func actionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "actions",
		Short: "Scaffold GitHub Actions workflows",
		Run: func(cmd *cobra.Command, args []string) {
			if err := runActionsCmd(); err != nil {
				PrintError(err)
			}
		},
	}

	return cmd
}

func runActionsCmd() error {
	cfg := config.New()

	m := tui.NewActionsModel(cfg)

	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running UI: %w", err)
	}

	return nil
}
