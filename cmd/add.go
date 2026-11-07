package cmd

import (
	"fmt"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/instructions"
	"github.com/spf13/cobra"
)

func addCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add",
		Short:   "Add a new instruction",
		Args:    cobra.ExactArgs(1),
		Example: `bark add "my-instruction"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := handleAddCmd(args[0]); err != nil {
				return err
			}

			fmt.Println("Instruction added")
			return nil
		},
	}

	return cmd
}

func handleAddCmd(name string) error {
	storage, err := config.GetStorage()
	if err != nil {
		return fmt.Errorf("error getting storage path: %w", err)
	}

	return instructions.Add(storage, name)
}
