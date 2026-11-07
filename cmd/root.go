package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/fang/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/version"
	"github.com/ionut-t/bark/v2/tui"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleRootCmd()
	},
}

func handleRootCmd() error {
	storage, err := config.GetStorage()
	if err != nil {
		return fmt.Errorf("error getting storage: %w", err)
	}

	m := tui.New(tui.Options{
		Storage: storage,
		Config:  config.New(),
	})

	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running UI: %w", err)
	}

	return nil
}

// Execute runs the CLI. Any returned error has already been printed to
// stderr; the caller only needs to set the exit status.
func Execute() error {
	if err := initConfig(); err != nil {
		printError(colorprofile.NewWriter(os.Stderr, os.Environ()), err)
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
	rootCmd.AddCommand(actionsCmd())

	rootCmd.PersistentFlags().Bool("plain", false, "Output plain text instead of TUI (auto-detected when stdout is piped)")
	rootCmd.Flags().StringP("config", "c", "", "config file (default is $HOME/.bark/config.toml)")

	return fang.Execute(
		context.Background(),
		rootCmd,
		fang.WithNotifySignal(os.Interrupt, os.Kill),
		fang.WithColorSchemeFunc(styles.FangColorScheme),
		fang.WithoutCompletions(),
		fang.WithErrorHandler(func(w io.Writer, _ fang.Styles, err error) {
			printError(w, err)
		}),
	)
}

func versionTemplate() string {
	versionTpl := styles.New(styles.IsDark()).Primary.Margin(0, 2).Render(logo) + `
  Version        %s
  Commit         %s
  Release date   %s
`
	return fmt.Sprintf(versionTpl, version.Version(), version.Commit(), version.Date())
}
