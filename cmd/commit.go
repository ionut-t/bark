package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ionut-t/bark/internal/config"
	"github.com/ionut-t/bark/tui"
	"github.com/ionut-t/coffee/styles"
	"github.com/spf13/cobra"
)

func commitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Generate a commit message and optionally create the commit",
		Run: func(cmd *cobra.Command, args []string) {
			if err := runCommitCmd(cmd); err != nil {
				fmt.Println(styles.Error.Render("Error: " + err.Error()))
			}
		},
	}

	cmd.Flags().BoolP("all", "a", false, "Include all changes")
	cmd.Flags().StringP("hint", "i", "", "Provide a hint for the commit message generation (e.g., 'feature/fix/docs')")

	return cmd
}

func runCommitCmd(cmd *cobra.Command) error {
	storage, err := config.GetStorage()
	if err != nil {
		return fmt.Errorf("error getting storage: %w", err)
	}

	all, _ := cmd.Flags().GetBool("all")
	hint, _ := cmd.Flags().GetString("hint")

	m := tui.New(tui.Options{
		Task:       tui.TaskCommit,
		Storage:    storage,
		Config:     config.New(),
		StagedOnly: !all,
		Hint:       hint,
	})

	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running UI: %w", err)
	}

	return nil
}
