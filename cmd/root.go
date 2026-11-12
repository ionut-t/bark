package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/fang/v2"
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

// configFunc returns the loaded config. Commands call it from RunE: the
// config is loaded in the root's PersistentPreRunE, after cobra has parsed
// --config.
type configFunc func() config.Config

// configLoader loads the config file at path, or the default file when path
// is empty.
type configLoader func(path string) (config.Config, error)

const configFlag = "config"

// newRootCmd builds the bark command tree. load runs once flags are parsed,
// before any command's RunE, so --config works before or after a subcommand.
func newRootCmd(load configLoader) *cobra.Command {
	var cfg config.Config
	getCfg := func() config.Config { return cfg }

	rootCmd := &cobra.Command{
		Use:  "bark",
		Long: "Get your code reviewed by legends, generate commit messages and PR descriptions",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			path, err := cmd.Flags().GetString(configFlag)
			if err != nil {
				return err
			}

			cfg, err = load(path)
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return handleRootCmd(getCfg())
		},
	}

	rootCmd.SetVersionTemplate(versionTemplate)

	rootCmd.AddCommand(configCmd(getCfg))
	rootCmd.AddCommand(reviewCmd(getCfg))
	rootCmd.AddCommand(commitCmd(getCfg))
	rootCmd.AddCommand(prCmd(getCfg))
	rootCmd.AddCommand(resetCmd())
	rootCmd.AddCommand(addCmd(getCfg))
	rootCmd.AddCommand(deleteCmd(getCfg))
	rootCmd.AddCommand(editCmd(getCfg))
	rootCmd.AddCommand(actionsCmd(getCfg))
	rootCmd.AddCommand(pricesCmd())

	// No -c shorthand: review (--changes) and edit (--commit) already use it.
	rootCmd.PersistentFlags().String(configFlag, "", "Config file (default is $HOME/.bark/.config.toml)")

	return rootCmd
}

func handleRootCmd(cfg config.Config) error {
	warnRepoLLM(os.Stderr, cfg)

	storage, err := config.GetStorage()
	if err != nil {
		return fmt.Errorf("error getting storage: %w", err)
	}

	m := tui.New(tui.Options{
		Storage: storage,
		Config:  cfg,
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
	return fang.Execute(
		context.Background(),
		newRootCmd(initConfig),
		fang.WithNotifySignal(os.Interrupt, os.Kill),
		fang.WithColorSchemeFunc(styles.FangColorScheme),
		fang.WithoutCompletions(),
		fang.WithErrorHandler(func(w io.Writer, _ fang.Styles, err error) {
			printError(w, err)
		}),
	)
}

// versionTemplate renders the banner when --version prints it, not when the
// command tree is built
const versionTemplate = "{{barkVersionBanner}}"

func init() {
	cobra.AddTemplateFunc("barkVersionBanner", versionBanner)
}

func versionBanner() string {
	versionTpl := styles.New(styles.IsDark()).Primary.Margin(0, 2).Render(logo) + `
  Version        %s
  Commit         %s
  Release date   %s
`
	return fmt.Sprintf(versionTpl, version.Version(), version.Commit(), version.Date())
}
