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

func commitCmd(getCfg configFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Generate a commit message and optionally create the commit",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommitCmd(cmd, getCfg())
		},
	}

	cmd.Flags().BoolP("all", "a", false, "Include all changes")
	cmd.Flags().StringP("hint", "i", "", "Provide a hint for the commit message generation (e.g., 'feature/fix/docs')")
	cmd.Flags().StringP("model", "m", "", "LLM model to use (overrides config)")
	cmd.Flags().StringP("provider", "P", "", "LLM provider to use (overrides config; by default inferred from the model name): "+strings.Join(config.SupportedProviders(), ", "))
	addPlainFlag(cmd)

	return cmd
}

func runCommitCmd(cmd *cobra.Command, cfg config.Config) error {
	all, _ := cmd.Flags().GetBool("all")
	hint, _ := cmd.Flags().GetString("hint")
	model, _ := cmd.Flags().GetString("model")
	provider, _ := cmd.Flags().GetString("provider")

	cfg.OverrideModel(model)

	if err := cfg.OverrideProvider(provider); err != nil {
		return fmt.Errorf("error overriding provider: %w", err)
	}
	warnRepoLLM(os.Stderr, cfg)

	// Read stdin if piped
	stdinDiff, err := readStdinIfPiped()
	if err != nil {
		return err
	}

	// Plain mode: stdin piped or --plain flag or stdout piped
	if stdinDiff != nil || isPlainMode(cmd) {
		return plain.RunCommit(plain.CommitOptions{
			Diff:   stdinDiff,
			All:    all,
			Hint:   hint,
			Config: cfg,
		})
	}

	// TUI mode
	storage, err := config.GetStorage()
	if err != nil {
		return fmt.Errorf("error getting storage: %w", err)
	}

	m := tui.New(tui.Options{
		Task:       tui.TaskCommit,
		Storage:    storage,
		Config:     cfg,
		StagedOnly: !all,
		Hint:       hint,
	})

	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running UI: %w", err)
	}

	return nil
}
