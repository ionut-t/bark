package cmd

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/tui"
	"github.com/spf13/cobra"
)

func actionsCmd(getCfg configFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "actions",
		Short: "Scaffold GitHub Actions workflows",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runActionsCmd(getCfg())
		},
	}

	return cmd
}

func runActionsCmd(cfg config.Config) error {
	m := tui.NewActionsModel(cfg)

	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running UI: %w", err)
	}

	return nil
}
