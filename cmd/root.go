package cmd

import (
	"context"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/fang"
	"github.com/ionut-t/bark/internal/config"
	"github.com/ionut-t/bark/internal/version"
	"github.com/ionut-t/bark/tui"
	"github.com/ionut-t/coffee/styles"
	"github.com/spf13/cobra"
)

const logo = `
__________               __    
\______   \_____ _______|  | __
 |    |  _/\__  \\_  __ \  |/ /
 |    |   \ / __ \|  | \/    < 
 |______  /(____  /__|  |__|_ \
        \/      \/           \/
`

var rootCmd = &cobra.Command{
	Use:  "bark",
	Long: "Get your code reviewed by legends, generate commit messages and PR descriptions",
	Run: func(cmd *cobra.Command, args []string) {
		if err := handleRootCmd(); err != nil {
			fmt.Println(styles.Error.Render("Error: " + err.Error()))
		}
	},
}

func handleRootCmd() error {
	storage, err := config.GetStorage()
	if err != nil {
		return fmt.Errorf("error getting storage: %w", err)
	}

	cfg, err := config.New()
	if err != nil {
		return fmt.Errorf("error creating config: %w", err)
	}

	m := tui.New(tui.Options{
		Storage: storage,
		Config:  cfg,
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

	rootCmd.SetVersionTemplate(versionTemplate())

	rootCmd.AddCommand(configCmd())
	rootCmd.AddCommand(reviewCmd())
	rootCmd.AddCommand(commitCmd())
	rootCmd.AddCommand(prCmd())
	rootCmd.AddCommand(resetCmd())
	rootCmd.AddCommand(addCmd())
	rootCmd.AddCommand(deleteCmd())
	rootCmd.AddCommand(editCmd())

	rootCmd.Flags().StringP("config", "c", "", "config file (default is $HOME/.bark/config.toml)")

	return fang.Execute(
		context.Background(),
		rootCmd,
		fang.WithNotifySignal(os.Interrupt, os.Kill),
		fang.WithColorSchemeFunc(styles.FangColorScheme),
		fang.WithoutCompletions(),
	)
}

func versionTemplate() string {
	versionTpl := styles.Primary.Margin(0, 2).Render(logo) + `
  Version        %s
  Commit         %s
  Release date   %s
`
	return fmt.Sprintf(versionTpl, version.Version(), version.Commit(), version.Date())
}
