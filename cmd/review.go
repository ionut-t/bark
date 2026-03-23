package cmd

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/internal/config"
	"github.com/ionut-t/bark/pkg/plain"
	"github.com/ionut-t/bark/tui"
	"github.com/spf13/cobra"
)

func reviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Review code changes",
		Run: func(cmd *cobra.Command, args []string) {
			if err := runReviewCmd(cmd); err != nil {
				if hasStdinData() || isPlainMode(cmd) {
					plain.Errf("%s", err)
				} else {
					PrintError(err)
				}
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
	cmd.Flags().String("hash", "", "Specify a commit hash to review")
	cmd.Flags().BoolP("stream", "S", false, "Stream the review output in real-time (only for plain mode)")

	cmd.MarkFlagsMutuallyExclusive("changes", "commit", "branch", "staged", "hash")

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
	hash, _ := cmd.Flags().GetString("hash")
	stream, _ := cmd.Flags().GetBool("stream")

	cfg := config.New()

	stdinDiff, err := readStdinIfPiped()
	if err != nil {
		return err
	}

	if stdinDiff != nil || isPlainMode(cmd) {
		return plain.RunReview(plain.ReviewOptions{
			Diff:            stdinDiff,
			ReviewerName:    reviewerName,
			Instruction:     instruction,
			SkipInstruction: skipInstruction,
			Storage:         storage,
			Config:          cfg,
			Staged:          staged,
			All:             changes,
			Branch:          branch,
			Hash:            hash,
			Stream:          stream,
		})
	}

	// TUI mode
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
		Config:          cfg,
		StagedOnly:      staged,
		SkipInstruction: skipInstruction,
		ReviewOption:    reviewOption,
	})

	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running UI: %w", err)
	}

	return nil
}
