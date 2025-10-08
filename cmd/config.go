package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/ionut-t/bark/internal/config"
	"github.com/ionut-t/bark/pkg/instructions"
	"github.com/ionut-t/bark/pkg/reviewers"
	"github.com/ionut-t/coffee/styles"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		Run: func(cmd *cobra.Command, args []string) {
			configPath := config.GetConfigFilePath()

			editorFlag, _ := cmd.Flags().GetString(config.EditorKey)
			llmProviderFlag, _ := cmd.Flags().GetString(config.LLMProviderKey)
			llmModelFlag, _ := cmd.Flags().GetString(config.LLMModelKey)

			flagsSet := false

			if editorFlag != "" {
				viper.Set(config.EditorKey, editorFlag)
				flagsSet = true
				fmt.Println(styles.Success.Render("Editor set to: " + editorFlag))
			}

			if llmProviderFlag != "" {
				viper.Set(config.LLMProviderKey, llmProviderFlag)
				flagsSet = true
				fmt.Println(styles.Success.Render("LLM provider set to: " + llmProviderFlag))
			}

			if llmModelFlag != "" {
				viper.Set(config.LLMModelKey, llmModelFlag)
				flagsSet = true
				fmt.Println(styles.Success.Render("LLM model set to: " + llmModelFlag))
			}

			if flagsSet {
				if err := viper.WriteConfig(); err != nil {
					fmt.Println(styles.Error.Render("error writing config: " + err.Error()))
				}
			} else {
				if err := openInEditor(configPath); err != nil {
					fmt.Println(styles.Error.Render("error opening editor: " + err.Error()))
				}
			}
		},
	}

	cmd.Flags().StringP(config.EditorKey, "e", "", "Set the editor to use for editing config")
	cmd.Flags().StringP(config.LLMProviderKey, "p", "", "Set the LLM provider (e.g., gemini, vertexai)")
	cmd.Flags().StringP(config.LLMModelKey, "m", "", "Set the LLM model")

	return cmd
}

func openInEditor(configPath string) error {
	editor := config.GetEditor()

	cmd := exec.Command(editor, configPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func initConfig() error {
	if _, err := config.InitialiseConfigFile(); err != nil {
		return fmt.Errorf("error initializing config: %w", err)
	}

	storage, err := config.GetStorage()

	if err != nil {
		return fmt.Errorf("error getting storage path: %w", err)
	}

	if err := reviewers.ConfigReviewers(storage, false); err != nil {
		return fmt.Errorf("error initializing reviewers: %w", err)
	}

	if err := instructions.ConfigInstructions(storage, false); err != nil {
		return fmt.Errorf("error initializing instructions: %w", err)
	}

	if err := config.InitialiseCommitInstructions(); err != nil {
		return fmt.Errorf("error initializing commit instructions: %w", err)
	}

	return nil
}
