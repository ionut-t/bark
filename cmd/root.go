package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/fang/v2"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/plain"
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
			if migratingDirs(cmd) {
				return nil
			}

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
	// fang replaces Version, and the template doesn't print it; setting it
	// here gives the root its --version flag before wrapFlagUsages runs.
	rootCmd.Version = version.Version()

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
	rootCmd.PersistentFlags().String(configFlag, "", "Config file (default is $XDG_CONFIG_HOME/bark/config.toml)")

	return rootCmd
}

func handleRootCmd(cfg config.Config) error {
	warnRepoLLM(os.Stderr, cfg)

	dirs, err := config.Paths()
	if err != nil {
		return err
	}

	return runTUI(tui.Options{
		Paths:  dirs,
		Config: cfg,
	})
}

// Execute runs the CLI. Any returned error has already been printed to
// stderr; the caller only needs to set the exit status.
func Execute() error {
	root := newRootCmd(initConfig)
	wrapFlagUsages(root, helpWidth())
	return fang.Execute(context.Background(), root, fangOptions()...)
}

// fangOptions configures how fang runs the command tree and prints its help
// and errors.
func fangOptions() []fang.Option {
	return []fang.Option{
		fang.WithNotifySignal(os.Interrupt, os.Kill),
		fang.WithColorSchemeFunc(styles.FangColorScheme),
		fang.WithoutCompletions(),
		fang.WithErrorHandler(func(w io.Writer, _ fang.Styles, err error) {
			printError(w, err)
		}),
	}
}

// ExitCode returns the exit status for err, the error Execute returned:
// plain.FailOnExitCode for a review that failed its fail_on threshold, 1
// for any other error, and 0 for none.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if _, ok := errors.AsType[*plain.FailOnError](err); ok {
		return plain.FailOnExitCode
	}
	return 1
}

// versionTemplate renders the banner when --version prints it, not when the
// command tree is built
const versionTemplate = "{{barkVersionBanner}}"

func init() {
	cobra.AddTemplateFunc("barkVersionBanner", versionBanner)
}

func versionBanner() string {
	var b strings.Builder
	b.WriteString(styles.New(styles.IsDark()).Primary.Margin(0, 2).Render(logo))
	b.WriteString("\n")
	writeVersionLine(&b, "Version", version.Version())
	writeVersionLine(&b, "Commit", version.Commit())
	writeVersionLine(&b, "Release date", version.Date())
	return b.String()
}

// writeVersionLine writes one banner row, skipping values the build didn't
// record.
func writeVersionLine(b *strings.Builder, label, value string) {
	if value == "" || value == "unknown" {
		return
	}
	fmt.Fprintf(b, "  %-15s%s\n", label, value)
}
