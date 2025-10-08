package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ionut-t/bark/internal/config"
	"github.com/ionut-t/bark/internal/utils"
	"github.com/ionut-t/bark/pkg/instructions"
	"github.com/ionut-t/bark/pkg/reviewers"
	"github.com/ionut-t/bark/tui"
	"github.com/ionut-t/coffee/styles"
	"github.com/spf13/cobra"
)

func editCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit an instruction or a reviewer",
		Example: `  bark edit -r "John Doe"
		bark edit -i "Code Quality Guidelines"
		bark edit -r // will show a list to select the reviewer to edit
		bark edit -i // will show a list to select the instruction to edit`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var name string
			if len(args) > 0 {
				name = args[0]
			}

			if err := handleEditCmd(cmd, name); err != nil {
				fmt.Println(styles.Error.Render("Error: " + err.Error()))
			}
		},
	}

	cmd.Flags().BoolP("reviewer", "r", false, "Edit a reviewer")
	cmd.Flags().BoolP("instruction", "i", false, "Edit an instruction")
	return cmd
}

func handleEditCmd(cmd *cobra.Command, name string) error {
	reviewer, _ := cmd.Flags().GetBool("reviewer")
	instruction, _ := cmd.Flags().GetBool("instruction")

	if !reviewer && !instruction {
		return fmt.Errorf("must specify either reviewer or instruction to edit")
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
		var assetPath string

		switch assetType {
		case tui.AssetInstruction:
			assetPath, err = instructions.GetPath(storage, name)

			if err != nil {
				return fmt.Errorf("error finding instruction: %w", err)
			}

		case tui.AssetReviewer:
			assetPath, err = reviewers.GetPath(storage, name)

			if err != nil {
				return fmt.Errorf("error finding reviewer: %w", err)
			}
		}

		return utils.OpenEditor(assetPath)
	}

	p := tea.NewProgram(tui.NewAssetsModel(storage, assetType, tui.AssetActionEdit), tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running UI: %w", err)
	}

	return nil
}
