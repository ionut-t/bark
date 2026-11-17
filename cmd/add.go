package cmd

import (
	"fmt"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/instructions"
	"github.com/spf13/cobra"
)

func addCmd(getCfg configFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add",
		Short:   "Add a new instruction",
		Args:    cobra.ExactArgs(1),
		Example: `bark add "my-instruction"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := handleAddCmd(getCfg(), args[0]); err != nil {
				return err
			}

			fmt.Println("Instruction added")
			return nil
		},
	}

	return cmd
}

func handleAddCmd(cfg config.Config, name string) error {
	dirs, err := config.Paths()
	if err != nil {
		return err
	}
	storage := dirs.Config

	return instructions.Add(storage, name, cfg.GetEditor())
}
