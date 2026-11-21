package cmd

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/findings"
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
	cmd.Flags().StringP("model", "m", "", "Model to use (overrides config)")
	cmd.Flags().StringP("provider", "P", "", "Model provider to use (overrides config; by default inferred from the model name): "+strings.Join(config.SupportedProviders(), ", "))
	cmd.Flags().BoolP("commit", "t", false, "Select commit to review")
	cmd.Flags().BoolP("changes", "c", false, "Review current changes")
	cmd.Flags().StringArrayP("instructions", "i", nil, "Custom instructions to guide the reviewer's feedback: a saved instruction name, file path or raw text (repeat to combine several)")
	cmd.Flags().StringP("branch", "b", "", "Provide a branch name to diff against the current branch")
	cmd.Flags().BoolP("staged", "s", false, "Review only staged changes")
	cmd.Flags().BoolP("skip-instruction", "k", false, "Skip the instructions selection step")
	cmd.Flags().String("hash", "", "Specify a commit hash to review")
	cmd.Flags().BoolP("stream", "S", false, "Stream the review output in real-time (only for plain mode)")
	cmd.Flags().StringP("pr", "p", "", "Review a GitHub pull request by number (requires gh CLI)")
	cmd.Flags().Uint32("max-diff-tokens", 0, "Maximum number of diff tokens to include in the prompt, estimated at 2.5 characters a token (0 disables the limit)")
	cmd.Flags().Uint32("max-diff-lines", 0, "Maximum number of diff lines to include in the prompt (0 disables the limit)")
	_ = cmd.Flags().MarkDeprecated("max-diff-lines", "use --max-diff-tokens instead")
	cmd.Flags().Bool("with-description", false, "Include the PR description in the review context (only applies with --pr)")
	cmd.Flags().Bool("with-context", false, "Include enclosing declarations (functions, structs, classes) and related code (definitions the change uses, callers of changed functions) as context for review")
	cmd.Flags().Bool("structured", false, "Ask for structured findings (file, line, severity, suggestion); the reviewer persona voices the intro, remarks and verdict")
	cmd.Flags().Bool("house-rules", true, "Read the repository's house rules (AGENTS.md and CLAUDE.md, or the house_rules_files config) into the review and hold the change to them (overrides config)")
	cmd.Flags().Bool("check", true, "Check structured findings with a second model call that reads the code around them, and set aside the ones that don't hold up (overrides config)")
	cmd.Flags().String("min-severity", "", "Show only structured findings of this severity or higher: "+strings.Join(config.Severities(), ", ")+" (overrides config)")
	cmd.Flags().String("fail-on", "", "Exit with status 3 when the review keeps a structured finding of this severity or higher: "+strings.Join(config.Severities(), ", ")+"; doubted findings and repeats of points dismissed on the pull request don't count, repeats of other points raised before do (overrides config; implies --structured and plain output)")
	addEffortFlag(cmd)
	cmd.Flags().Bool("comment", false, "Post the review to the pull request, with findings as inline comments and fixes as suggestions (requires --pr; implies --structured)")
	cmd.Flags().Bool("approve", false, "Post the review as an approval when it keeps no finding at or above --approve-below and no earlier point on the pull request may still be open (requires --comment)")
	cmd.Flags().String("approve-below", "", "With --approve, approve only when every finding is below this severity: "+strings.Join(config.Severities(), ", ")+" (default medium, so low findings are left as comments on the approval; a lower --fail-on wins)")
	cmd.Flags().Bool("no-resolve", false, "With --comment, leave open the threads of Bark's earlier inline comments that its check finds fixed (by default Bark replies on each with the reason and resolves it)")
	cmd.Flags().Bool("full", false, "Review the whole pull request, not only the changes since Bark's last posted review of it (requires --pr; matters with --comment)")
	addPlainFlag(cmd)

	cmd.MarkFlagsMutuallyExclusive("changes", "commit", "branch", "staged", "hash", "pr")

	return cmd
}

func runReviewCmd(cmd *cobra.Command, cfg config.Config) error {
	dirs, err := config.Paths()
	if err != nil {
		return err
	}

	reviewerName, _ := cmd.Flags().GetString("as")
	commit, _ := cmd.Flags().GetBool("commit")
	changes, _ := cmd.Flags().GetBool("changes")
	instructions, _ := cmd.Flags().GetStringArray("instructions")
	branch, _ := cmd.Flags().GetString("branch")
	staged, _ := cmd.Flags().GetBool("staged")
	skipInstruction, _ := cmd.Flags().GetBool("skip-instruction")
	hash, _ := cmd.Flags().GetString("hash")
	stream, _ := cmd.Flags().GetBool("stream")
	pr, _ := cmd.Flags().GetString("pr")
	withDescription, _ := cmd.Flags().GetBool("with-description")
	model, _ := cmd.Flags().GetString("model")
	provider, _ := cmd.Flags().GetString("provider")
	comment, _ := cmd.Flags().GetBool("comment")
	full, _ := cmd.Flags().GetBool("full")
	approve, _ := cmd.Flags().GetBool("approve")
	approveBelow, _ := cmd.Flags().GetString("approve-below")
	noResolve, _ := cmd.Flags().GetBool("no-resolve")

	if withDescription && pr == "" {
		return fmt.Errorf("--with-description requires --pr")
	}

	if comment && pr == "" {
		return fmt.Errorf("--comment requires --pr")
	}

	if approve && !comment {
		return fmt.Errorf("--approve requires --comment")
	}

	if noResolve && !comment {
		return fmt.Errorf("--no-resolve requires --comment")
	}

	if cmd.Flags().Changed("approve-below") && !approve {
		return fmt.Errorf("--approve-below requires --approve")
	}
	approveBelow, err = config.NormaliseApproveBelow(approveBelow)
	if err != nil {
		return err
	}

	if full && pr == "" {
		return fmt.Errorf("--full requires --pr")
	}

	stdinDiff, err := readStdinIfPiped()
	if err != nil {
		return err
	}

	cfg.OverrideModel(model)

	if err := cfg.OverrideProvider(provider); err != nil {
		return fmt.Errorf("error overriding provider: %w", err)
	}

	if cmd.Flags().Changed("max-diff-tokens") {
		maxDiffTokens, _ := cmd.Flags().GetUint32("max-diff-tokens")
		cfg.OverrideMaxDiffTokens(maxDiffTokens)
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

	if cmd.Flags().Changed("house-rules") {
		houseRules, _ := cmd.Flags().GetBool("house-rules")
		cfg.OverrideHouseRules(houseRules)
	}

	if cmd.Flags().Changed("check") {
		check, _ := cmd.Flags().GetBool("check")
		cfg.OverrideCheckFindings(check)
	}

	if cmd.Flags().Changed("min-severity") {
		minSeverity, _ := cmd.Flags().GetString("min-severity")
		if err := cfg.OverrideMinSeverity(minSeverity); err != nil {
			return err
		}
	}
	if err := overrideEffort(cmd, cfg); err != nil {
		return err
	}
	// failOn is the --fail-on flag only: a fail_on from a config file
	// applies when the run is plain anyway, and the TUI ignores it.
	failOn := ""
	if cmd.Flags().Changed("fail-on") {
		failOn, _ = cmd.Flags().GetString("fail-on")
		if err := cfg.OverrideFailOn(failOn); err != nil {
			return err
		}
	}
	warnRepoLLM(os.Stderr, cfg)

	// Posting and failing on findings run in plain mode: the TUI has no
	// step for posting, and its exit status means nothing.
	if stdinDiff != nil || comment || failOn != "" || isPlainMode(cmd) {
		return plain.RunReview(plain.ReviewOptions{
			Diff:              stdinDiff,
			ReviewerName:      reviewerName,
			Instructions:      instructions,
			SkipInstruction:   skipInstruction,
			Paths:             dirs,
			Config:            cfg,
			Staged:            staged,
			All:               changes,
			Branch:            branch,
			Hash:              hash,
			Stream:            stream,
			PR:                pr,
			WithPRDescription: withDescription,
			Comment:           comment,
			Full:              full,
			Approve:           approve,
			ApproveBelow:      findings.Severity(approveBelow),
			NoResolve:         noResolve,
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
		Paths:             dirs,
		ReviewerName:      reviewerName,
		Instructions:      instructions,
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
