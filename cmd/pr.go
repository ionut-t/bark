package cmd

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/plain"
	"github.com/ionut-t/bark/v2/tui"
	"github.com/spf13/cobra"
)

func prCmd(getCfg configFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr",
		Short: "Generate a pull request description",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPRCmd(cmd, getCfg())
		},
	}

	cmd.Flags().StringP("branch", "b", "", "The base branch to compare against (optional)")
	cmd.Flags().StringP("pr", "p", "", "Generate a description for a GitHub pull request by number (requires gh CLI)")
	cmd.Flags().StringP("model", "m", "", "LLM model to use (overrides config)")
	cmd.Flags().StringP("provider", "P", "", "LLM provider to use (overrides config): "+strings.Join(config.SupportedProviders(), ", "))
	cmd.Flags().StringP("instructions", "i", "", "Custom instructions (file path or raw text, overrides default PR instructions)")
	cmd.Flags().Uint32("max-diff-lines", 0, "Maximum number of diff lines to include in the prompt (0 disables the limit)")
	addPlainFlag(cmd)

	cmd.MarkFlagsMutuallyExclusive("branch", "pr")

	return cmd
}

func runPRCmd(cmd *cobra.Command, cfg config.Config) error {
	branch, _ := cmd.Flags().GetString("branch")
	pr, _ := cmd.Flags().GetString("pr")
	model, _ := cmd.Flags().GetString("model")
	provider, _ := cmd.Flags().GetString("provider")
	instructions, _ := cmd.Flags().GetString("instructions")

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
	warnRepoLLM(os.Stderr, cfg)

	if stdinDiff != nil || isPlainMode(cmd) {
		return plain.RunPR(plain.PROptions{
			Diff:         stdinDiff,
			Branch:       branch,
			PR:           pr,
			Instructions: instructions,
			Config:       cfg,
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
