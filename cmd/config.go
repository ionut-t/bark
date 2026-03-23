package cmd

import (
	"fmt"

	"github.com/ionut-t/bark/internal/config"
	"github.com/ionut-t/bark/internal/utils"
	"github.com/ionut-t/bark/pkg/instructions"
	"github.com/ionut-t/bark/pkg/reviewers"
	"github.com/spf13/cobra"
)

func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := config.New()
			configPath := config.GetConfigFilePath()

			editorFlag, _ := cmd.Flags().GetString(config.EditorKey)
			llmProviderFlag, _ := cmd.Flags().GetString(config.LLMProviderKey)
			llmModelFlag, _ := cmd.Flags().GetString(config.LLMModelKey)
			maxDiffLinesFlag, _ := cmd.Flags().GetUint32(config.MaxDiffLinesKey)

			flagsSet := false

			if editorFlag != "" {
				if err := cfg.SetEditor(editorFlag); err != nil {
					PrintError(err)
					return
				}
				flagsSet = true
			}

			if llmProviderFlag != "" {
				if err := cfg.SetLLMProvider(llmProviderFlag); err != nil {
					PrintError(err)
					return
				}
				flagsSet = true
			}

			if llmModelFlag != "" {
				if err := cfg.SetLLMModel(llmModelFlag); err != nil {
					PrintError(err)
					return
				}
				flagsSet = true
			}

			if maxDiffLinesFlag != 0 {
				if err := cfg.SetMaxDiffLines(maxDiffLinesFlag); err != nil {
					PrintError(err)
					return
				}
				flagsSet = true
			}

			if !flagsSet {
				if err := utils.OpenEditor(configPath); err != nil {
					PrintError(err)
				}
			}
		},
	}

	cmd.Flags().StringP(config.EditorKey, "e", "", "Set the editor to use for editing config")
	cmd.Flags().StringP(config.LLMProviderKey, "p", "", "Set the LLM provider (e.g., gemini, vertexai)")
	cmd.Flags().StringP(config.LLMModelKey, "m", "", "Set the LLM model")
	cmd.Flags().Uint32P(config.MaxDiffLinesKey, "d", 0, fmt.Sprintf("Set the maximum number of diff lines to include in the prompt (default: %d)", config.DEFAULT_MAX_DIFF_LINES))

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
