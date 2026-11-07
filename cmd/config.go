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
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.New()
			configPath := config.GetConfigFilePath()

			editorFlag, _ := cmd.Flags().GetString(config.EditorKey)
			llmProviderFlag, _ := cmd.Flags().GetString(config.LLMProviderKey)
			llmModelFlag, _ := cmd.Flags().GetString(config.LLMModelKey)
			maxDiffLinesFlag, _ := cmd.Flags().GetUint32(config.MaxDiffLinesKey)

			flagsSet := false

			if editorFlag != "" {
				if err := cfg.SetEditor(editorFlag); err != nil {
					return err
				}
				flagsSet = true
			}

			if llmProviderFlag != "" {
				if err := cfg.SetLLMProvider(llmProviderFlag); err != nil {
					return err
				}
				flagsSet = true
			}

			if llmModelFlag != "" {
				if err := cfg.SetLLMModel(llmModelFlag); err != nil {
					return err
				}
				flagsSet = true
			}

			if maxDiffLinesFlag != 0 {
				if err := cfg.SetMaxDiffLines(maxDiffLinesFlag); err != nil {
					return err
				}
				flagsSet = true
			}

			if cmd.Flags().Changed(config.ContextEnrichmentKey) {
				contextEnrichmentFlag, _ := cmd.Flags().GetBool(config.ContextEnrichmentKey)
				if err := cfg.SetContextEnrichment(contextEnrichmentFlag); err != nil {
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

	cmd.Flags().StringP(config.EditorKey, "e", "", "Set the editor to use for editing config")
	cmd.Flags().StringP(config.LLMProviderKey, "p", "", "Set the LLM provider: "+strings.Join(config.SupportedProviders(), ", "))
	cmd.Flags().StringP(config.LLMModelKey, "m", "", "Set the LLM model")
	cmd.Flags().Uint32P(config.MaxDiffLinesKey, "d", 0, fmt.Sprintf("Set the maximum number of diff lines to include in the prompt (default: %d)", config.DEFAULT_MAX_DIFF_LINES))
	cmd.Flags().Bool(config.ContextEnrichmentKey, false, "Enable or disable enclosing-declaration context for reviews (default: false)")

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
