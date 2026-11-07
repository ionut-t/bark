package cmd

import (
	"fmt"
	"strings"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/instructions"
	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/ionut-t/bark/v2/internal/utils"
	"github.com/spf13/cobra"
)

func configCmd() *cobra.Command {
	var (
		editor            string
		provider          string
		model             string
		maxDiffLines      uint32
		contextEnrichment bool
	)

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.New()
			configPath := config.GetConfigFilePath()

			flagsSet := false

			if editor != "" {
				if err := cfg.SetEditor(editor); err != nil {
					return err
				}
				flagsSet = true
			}

			if provider != "" {
				if err := cfg.SetLLMProvider(provider); err != nil {
					return err
				}
				flagsSet = true
			}

			if model != "" {
				if err := cfg.SetLLMModel(model); err != nil {
					return err
				}
				flagsSet = true
			}

			if maxDiffLines != 0 {
				if err := cfg.SetMaxDiffLines(maxDiffLines); err != nil {
					return err
				}
				flagsSet = true
			}

			if cmd.Flags().Changed("context-enrichment") || cmd.Flags().Changed(config.ContextEnrichmentKey) {
				if err := cfg.SetContextEnrichment(contextEnrichment); err != nil {
					return err
				}
				flagsSet = true
			}

			if !flagsSet {
				return utils.OpenEditor(configPath)
			}

			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&editor, "editor", "e", "", "Set the editor to use for editing config")
	flags.StringVarP(&provider, "provider", "p", "", "Set the LLM provider: "+strings.Join(config.SupportedProviders(), ", "))
	flags.StringVarP(&model, "model", "m", "", "Set the LLM model")
	flags.Uint32VarP(&maxDiffLines, "max-diff-lines", "d", 0, fmt.Sprintf("Set the maximum number of diff lines to include in the prompt (default: %d)", config.DEFAULT_MAX_DIFF_LINES))
	flags.BoolVar(&contextEnrichment, "context-enrichment", false, "Enable or disable enclosing-declaration context for reviews (default: false)")

	// Old names, matching the config file keys. Kept so existing scripts keep
	// working; hidden from help, and pflag prints a deprecation notice.
	flags.StringVar(&provider, config.LLMProviderKey, "", "")
	flags.StringVar(&model, config.LLMModelKey, "", "")
	flags.Uint32Var(&maxDiffLines, config.MaxDiffLinesKey, 0, "")
	flags.BoolVar(&contextEnrichment, config.ContextEnrichmentKey, false, "")
	for oldName, newName := range map[string]string{
		config.LLMProviderKey:       "provider",
		config.LLMModelKey:          "model",
		config.MaxDiffLinesKey:      "max-diff-lines",
		config.ContextEnrichmentKey: "context-enrichment",
	} {
		_ = flags.MarkDeprecated(oldName, "use --"+newName+" instead")
	}

	return cmd
}

func initConfig() error {
	if _, err := config.InitialiseConfigFile(); err != nil {
		return fmt.Errorf("error initializing config: %w", err)
	}

	storage, err := config.GetStorage()
	if err != nil {
		return fmt.Errorf("error getting storage path: %w", err)
	}

	if err := reviewers.Config(storage, false); err != nil {
		return fmt.Errorf("error initializing reviewers: %w", err)
	}

	if err := instructions.Config(storage, false); err != nil {
		return fmt.Errorf("error initializing instructions: %w", err)
	}

	if err := config.InitialiseCommitInstructions(); err != nil {
		return fmt.Errorf("error initializing commit instructions: %w", err)
	}

	return nil
}
