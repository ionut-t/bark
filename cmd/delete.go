package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ionut-t/bark/internal/config"
	"github.com/ionut-t/bark/pkg/instructions"
	"github.com/ionut-t/bark/pkg/reviewers"
	"github.com/ionut-t/bark/tui"
	"github.com/ionut-t/coffee/styles"
	"github.com/spf13/cobra"
)

func deleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete an instruction or a reviewer",
		Example: `  bark delete -r "John Doe"
		bark delete -i "Code Quality Guidelines"
		bark delete -r // will show a list to select the reviewer to remove
		bark delete -i // will show a list to select the instruction to remove`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var name string
			if len(args) > 0 {
				name = args[0]
			}

			if err := handleDeleteCmd(cmd, name); err != nil {
				fmt.Println(styles.Error.Render("Error: " + err.Error()))
			}
		},
	}

	cmd.Flags().BoolP("reviewer", "r", false, "Delete a reviewer")
	cmd.Flags().BoolP("instruction", "i", false, "Delete an instruction")
	return cmd
}

func handleDeleteCmd(cmd *cobra.Command, name string) error {
	reviewer, _ := cmd.Flags().GetBool("reviewer")
	instruction, _ := cmd.Flags().GetBool("instruction")

	if !reviewer && !instruction {
		return fmt.Errorf("must specify either reviewer or instruction to delete")
	}

	storage, err := config.GetStorage()
	if err != nil {
		return fmt.Errorf("error getting storage: %w", err)
	}

	var assetType tui.AssetType

	if reviewer {
		assetType = tui.AssetReviewer
	} else if instruction {
		assetType = tui.AssetInstruction
	}

	if name != "" {
		switch assetType {
		case tui.AssetInstruction:
			if err := instructions.Delete(storage, name); err != nil {
				return fmt.Errorf("error deleting instruction: %w", err)
			}
			return nil

		case tui.AssetReviewer:
			if err := reviewers.Delete(storage, name); err != nil {
				return fmt.Errorf("error deleting reviewer: %w", err)
			}
			return nil
		}
	}

	p := tea.NewProgram(tui.NewAssetsModel(storage, assetType, tui.AssetActionDelete), tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running UI: %w", err)
	}

	return nil
}
