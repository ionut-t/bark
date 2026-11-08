package cmd

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/plain"
	"github.com/ionut-t/bark/v2/tui"
	"github.com/spf13/cobra"
)

func reviewCmd(getCfg configFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Review code changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReviewCmd(cmd, getCfg())
		},
	}

	cmd.Flags().String("as", "", "Specify the reviewer to use directly")
	cmd.Flags().StringP("model", "m", "", "LLM model to use (overrides config)")
	cmd.Flags().StringP("provider", "P", "", "LLM provider to use (overrides config): "+strings.Join(config.SupportedProviders(), ", "))
	cmd.Flags().BoolP("commit", "t", false, "Select commit to review")
	cmd.Flags().BoolP("changes", "c", false, "Review current changes")
	cmd.Flags().StringP("instructions", "i", "", "Custom instructions to guide the reviewer's feedback")
	cmd.Flags().StringP("branch", "b", "", "Provide a branch name to diff against the current branch")
	cmd.Flags().BoolP("staged", "s", false, "Review only staged changes")
	cmd.Flags().BoolP("skip-instruction", "k", false, "Skip the instructions selection step")
	cmd.Flags().String("hash", "", "Specify a commit hash to review")
	cmd.Flags().BoolP("stream", "S", false, "Stream the review output in real-time (only for plain mode)")
	cmd.Flags().StringP("pr", "p", "", "Review a GitHub pull request by number (requires gh CLI)")
	cmd.Flags().Uint32("max-diff-lines", 0, "Maximum number of diff lines to include in the prompt (0 disables the limit)")
	cmd.Flags().Bool("with-description", false, "Include the PR description in the review context (only applies with --pr)")
	cmd.Flags().Bool("with-context", false, "Include enclosing declarations (functions, structs, classes) as context for review")
	cmd.Flags().Bool("structured", false, "Ask for structured findings (file, line, severity, suggestion); the reviewer persona voices the intro, remarks and verdict")

	cmd.MarkFlagsMutuallyExclusive("changes", "commit", "branch", "staged", "hash", "pr")

	return cmd
}

func runReviewCmd(cmd *cobra.Command, cfg config.Config) error {
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
	pr, _ := cmd.Flags().GetString("pr")
	withDescription, _ := cmd.Flags().GetBool("with-description")
	model, _ := cmd.Flags().GetString("model")
	provider, _ := cmd.Flags().GetString("provider")

	if withDescription && pr == "" {
		return fmt.Errorf("--with-description requires --pr")
	}

	stdinDiff, err := readStdinIfPiped()
	if err != nil {
		return err
	}

	cfg.OverrideModel(model)

	if err := cfg.OverrideProvider(provider); err != nil {
		return fmt.Errorf("error overriding provider: %w", err)
	}

	if cmd.Flags().Changed("max-diff-lines") {
		maxDiffLines, _ := cmd.Flags().GetUint32("max-diff-lines")
		cfg.OverrideMaxDiffLines(maxDiffLines)
	}

	if cmd.Flags().Changed("with-context") {
		contextEnrich, _ := cmd.Flags().GetBool("with-context")
		cfg.OverrideContextEnrichment(contextEnrich)
	}

	if cmd.Flags().Changed("structured") {
		structured, _ := cmd.Flags().GetBool("structured")
		cfg.OverrideStructuredFindings(structured)
	}

	if stdinDiff != nil || isPlainMode(cmd) {
		return plain.RunReview(plain.ReviewOptions{
			Diff:              stdinDiff,
			ReviewerName:      reviewerName,
			Instruction:       instruction,
			SkipInstruction:   skipInstruction,
			Storage:           storage,
			Config:            cfg,
			Staged:            staged,
			All:               changes,
			Branch:            branch,
			Hash:              hash,
			Stream:            stream,
			PR:                pr,
			WithPRDescription: withDescription,
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
	} else if pr != "" {
		reviewOption = tui.ReviewPR
	}

	m := tui.New(tui.Options{
		Task:              tui.TaskReview,
		Storage:           storage,
		ReviewerName:      reviewerName,
		Instruction:       instruction,
		Branch:            branch,
		SelectCommit:      commit,
		Config:            cfg,
		StagedOnly:        staged,
		SkipInstruction:   skipInstruction,
		ReviewOption:      reviewOption,
		PR:                pr,
		WithPRDescription: withDescription,
	})

	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running UI: %w", err)
	}

	return nil
}
