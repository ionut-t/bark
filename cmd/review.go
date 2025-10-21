package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ionut-t/bark/internal/config"
	"github.com/ionut-t/bark/tui"
	"github.com/ionut-t/coffee/styles"
	"github.com/spf13/cobra"
)

func reviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Review code changes",
		Run: func(cmd *cobra.Command, args []string) {
			if err := runReviewCmd(cmd); err != nil {
				fmt.Println(styles.Error.Render("Error: " + err.Error()))
			}
		},
	}

	cmd.Flags().String("as", "", "Specify the reviewer to use directly")
	cmd.Flags().BoolP("commit", "t", false, "Select commit to review")
	cmd.Flags().BoolP("changes", "c", false, "Review current changes")
	cmd.Flags().StringP("instructions", "i", "", "Custom instructions to guide the reviewer's feedback")
	cmd.Flags().StringP("branch", "b", "", "Provide a branch name to diff against the current branch")
	cmd.Flags().BoolP("staged", "s", false, "Review only staged changes")
	cmd.Flags().BoolP("skip-instruction", "k", false, "Skip the instructions selection step")

	cmd.MarkFlagsMutuallyExclusive("changes", "commit", "branch", "staged")

	return cmd
}

func runReviewCmd(cmd *cobra.Command) error {
	storage, err := config.GetStorage()
	if err != nil {
		return fmt.Errorf("error getting storage: %w", err)
	}

	reviewerName, _ := cmd.Flags().GetString("as")
	commit, _ := cmd.Flags().GetBool("commit")
	changes, _ := cmd.Flags().GetBool("changes")
	instruction, _ := cmd.Flags().GetString("instructions")
	branch, _ := cmd.Flags().GetString("branch")
	staged, _ := cmd.Flags().GetBool("staged")
	skipInstruction, _ := cmd.Flags().GetBool("skip-instruction")

	var reviewOption tui.ReviewOption
	if changes {
		reviewOption = tui.ReviewOptionCurrentChanges
	} else if staged {
		reviewOption = tui.ReviewOptionStagedChanges
	} else if commit {
		reviewOption = tui.ReviewOptionCommit
	} else if branch != "" {
		reviewOption = tui.ReviewOptionBranch
	}

	m := tui.New(tui.Options{
		Task:            tui.TaskReview,
		Storage:         storage,
		ReviewerName:    reviewerName,
		Instruction:     instruction,
		Branch:          branch,
		SelectCommit:    commit,
		Config:          config.New(),
		StagedOnly:      staged,
		SkipInstruction: skipInstruction,
		ReviewOption:    reviewOption,
	})

	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running UI: %w", err)
	}

	return nil
}
