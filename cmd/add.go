package cmd

import (
	"fmt"

	"github.com/ionut-t/bark/internal/config"
	"github.com/ionut-t/bark/pkg/instructions"
	"github.com/spf13/cobra"
)

func addCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add",
		Short:   "Add a new instruction",
		Args:    cobra.ExactArgs(1),
		Example: `bark add "my-instruction"`,
		Run: func(cmd *cobra.Command, args []string) {
			err := handleAddCmd(args[0])
			if err != nil {
				PrintError(err)
				return
			}

			fmt.Println("Instruction added")
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
