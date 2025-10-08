package cmd

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/fang"
	"github.com/ionut-t/bark/internal/config"
	"github.com/ionut-t/bark/tui"
	"github.com/ionut-t/coffee/styles"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "bark",
	Short: "A brief description of your application",
	Long: `Get your code reviewed by legends.
	
bark is a TUI that lets you review pull requests and commits through the lens of legendary developers 
and personalities. Want Linus Torvalds to tear apart your PR? Shakespeare to write a sonnet about your bugs? Gordon Ramsay to roast your spaghetti code? Choose your reviewer and get AI-powered feedback in their authentic voice—brutal honesty included.

Terminal-native. Works with any language. No sugarcoating. Just the real review your code deserves.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := handleRootCmd(cmd); err != nil {
			fmt.Println(styles.Error.Render("Error: " + err.Error()))
		}
	},
}

func handleRootCmd(cmd *cobra.Command) error {
	storage, err := config.GetStorage()
	if err != nil {
		return fmt.Errorf("error getting storage: %w", err)
	}

	cfg, err := config.New()
	if err != nil {
		return fmt.Errorf("error creating config: %w", err)
	}

	reviewerName, _ := cmd.Flags().GetString("as")
	commit, _ := cmd.Flags().GetBool("commit")
	instruction, _ := cmd.Flags().GetString("instructions")
	branch, _ := cmd.Flags().GetString("branch")
	staged, _ := cmd.Flags().GetBool("staged")

	m := tui.New(tui.Options{
		Storage:      storage,
		ReviewerName: reviewerName,
		Instruction:  instruction,
		Branch:       branch,
		SelectCommit: commit,
		Config:       cfg,
		StagedOnly:   staged,
	})

	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running UI: %w", err)
	}

	return nil
}

func Execute() error {
	if err := initConfig(); err != nil {
		return err
	}

	rootCmd.AddCommand(configCmd())
	rootCmd.AddCommand(resetCmd())
	rootCmd.AddCommand(addCmd())
	rootCmd.AddCommand(deleteCmd())
	rootCmd.AddCommand(editCmd())

	rootCmd.Flags().StringP("config", "c", "", "config file (default is $HOME/.bark/config.toml)")
	rootCmd.Flags().StringP("as", "r", "", "Specify the reviewer to use directly")
	rootCmd.Flags().BoolP("commit", "t", false, "Select commit to review")
	rootCmd.Flags().StringP("instructions", "i", "", "Custom instructions to guide the reviewer's feedback")
	rootCmd.Flags().StringP("branch", "b", "", "Provide a branch name to diff against the current branch")
	rootCmd.Flags().BoolP("staged", "s", false, "Review only staged changes")

	return fang.Execute(
		context.Background(),
		rootCmd,
		fang.WithNotifySignal(os.Interrupt, os.Kill),
		fang.WithColorSchemeFunc(styles.FangColorScheme),
		fang.WithoutCompletions(),
	)
}
