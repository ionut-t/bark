package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ionut-t/bark/internal/config"
	"github.com/ionut-t/bark/tui"
	"github.com/ionut-t/coffee/styles"
	"github.com/spf13/cobra"
)

func prCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr",
		Short: "Generate a pull request message",
		Run: func(cmd *cobra.Command, args []string) {
			if err := runPRCmd(cmd); err != nil {
				fmt.Println(styles.Error.Render("Error: " + err.Error()))
			}
		},
	}

	cmd.Flags().StringP("branch", "b", "", "The base branch to compare against (optional)")

	return cmd
}

func runPRCmd(cmd *cobra.Command) error {
	storage, err := config.GetStorage()
	if err != nil {
		return fmt.Errorf("error getting storage: %w", err)
	}

	branch, _ := cmd.Flags().GetString("branch")

	m := tui.New(tui.Options{
		Task:    tui.TaskPRDescription,
		Storage: storage,
		Config:  config.New(),
		Branch:  branch,
	})

	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running UI: %w", err)
	}

	return nil
}
