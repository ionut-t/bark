package cmd

import (
	"fmt"

	"github.com/ionut-t/bark/internal/config"
	"github.com/ionut-t/bark/pkg/instructions"
	"github.com/ionut-t/bark/pkg/reviewers"
	"github.com/ionut-t/coffee/styles"
	"github.com/spf13/cobra"
)

func resetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Reset reviewers and/or instructions to default",
		Long: `Reset reviewers and/or instructions to their default state by overwriting any existing custom files.
This does not affect any files that you may have created or modified outside of the default set, unless you use the --hard flag.
		`,
		Run: func(cmd *cobra.Command, args []string) {
			msg, err := handleReset(cmd)

			if err != nil {
				fmt.Println(styles.Error.Render("Error: " + err.Error()))
				return
			}

			fmt.Println(styles.Success.Render(msg))
		},
	}

	cmd.Flags().BoolP("reviewers", "r", false, "Reset reviewers to default")
	cmd.Flags().BoolP("instructions", "i", false, "Reset instructions to default")
	cmd.Flags().Bool("hard", false, "Hard reset: remove all custom files and re-download defaults")

	return cmd
}

func handleReset(cmd *cobra.Command) (string, error) {
	storage, err := config.GetStorage()

	if err != nil {
		return "", fmt.Errorf("error getting storage path: %w", err)
	}

	resetReviewers, _ := cmd.Flags().GetBool("reviewers")
	resetInstructions, _ := cmd.Flags().GetBool("instructions")
	hardReset, _ := cmd.Flags().GetBool("hard")

	if !resetReviewers && !resetInstructions {
		return "", fmt.Errorf("specify at least one of --reviewers or --instructions to reset")
	}

	if resetReviewers {
		if hardReset {
			if err := reviewers.RemoveDir(storage); err != nil {
				return "", fmt.Errorf("error performing hard reset of reviewers: %w", err)
			}
		}

		if err := reviewers.Config(storage, true); err != nil {
			return "", fmt.Errorf("error resetting reviewers: %w", err)
		}
	}

	if resetInstructions {
		if hardReset {
			if err := instructions.RemoveDir(storage); err != nil {
				return "", fmt.Errorf("error performing hard reset of instructions: %w", err)
			}
		}

		if err := instructions.Config(storage, true); err != nil {
			return "", fmt.Errorf("error resetting instructions: %w", err)
		}

	}

	if resetReviewers && resetInstructions {
		return "Reviewers and instructions have been reset to default", nil
	}

	if resetReviewers {
		return "Reviewers have been reset to default", nil
	}

	if resetInstructions {
		return "Instructions have been reset to default", nil
	}

	return "", nil
}
