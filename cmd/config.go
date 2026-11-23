package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/ionut-t/bark/v2/internal/instructions"
	"github.com/ionut-t/bark/v2/internal/paths"
	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/ionut-t/bark/v2/internal/utils"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func configCmd(getCfg configFunc) *cobra.Command {
	var (
		editor            string
		provider          string
		model             string
		maxDiffTokens     uint32
		maxDiffLines      uint32
		contextEnrichment bool
		structured        bool
		checkFindings     bool
		houseRules        bool
		minSeverity       string
		failOn            string
		effort            string
		timeout           string
		migrate           bool
	)

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if migrate {
				if err := migrateAlone(cmd.Flags()); err != nil {
					return err
				}
				return migrateDirs(paths.System(), time.Now(), cmd.OutOrStdout())
			}

			cfg := getCfg()
			flagsSet := false

			if editor != "" {
				if err := cfg.SetEditor(editor); err != nil {
					return err
				}
				flagsSet = true
			}

			if provider != "" {
				if err := cfg.SetLLMProvider(provider); err != nil {
					return err
				}
				flagsSet = true
			}

			if model != "" {
				if err := cfg.SetLLMModel(model); err != nil {
					return err
				}
				flagsSet = true
			}

			if cmd.Flags().Changed("max-diff-tokens") {
				if err := cfg.SetMaxDiffTokens(maxDiffTokens); err != nil {
					return err
				}
				flagsSet = true
			}

			if anyChanged(cmd.Flags(), "max-diff-lines", config.MaxDiffLinesKey) {
				if err := cfg.SetMaxDiffLines(maxDiffLines); err != nil {
					return err
				}
				flagsSet = true
			}

			if anyChanged(cmd.Flags(), "context-enrichment", config.ContextEnrichmentKey) {
				if err := cfg.SetContextEnrichment(contextEnrichment); err != nil {
					return err
				}
				flagsSet = true
			}

			if cmd.Flags().Changed("structured-findings") {
				if err := cfg.SetStructuredFindings(structured); err != nil {
					return err
				}
				flagsSet = true
			}

			if cmd.Flags().Changed("check-findings") {
				if err := cfg.SetCheckFindings(checkFindings); err != nil {
					return err
				}
				flagsSet = true
			}

			if cmd.Flags().Changed("house-rules") {
				if err := cfg.SetHouseRules(houseRules); err != nil {
					return err
				}
				flagsSet = true
			}

			if cmd.Flags().Changed("min-severity") {
				if err := cfg.SetMinSeverity(minSeverity); err != nil {
					return err
				}
				flagsSet = true
			}

			if cmd.Flags().Changed("fail-on") {
				if err := cfg.SetFailOn(failOn); err != nil {
					return err
				}
				flagsSet = true
			}

			if cmd.Flags().Changed("effort") {
				if err := cfg.SetEffort(effort); err != nil {
					return err
				}
				flagsSet = true
			}

			if cmd.Flags().Changed("timeout") {
				if err := cfg.SetLLMTimeout(timeout); err != nil {
					return err
				}
				flagsSet = true
			}

			if !flagsSet {
				return utils.OpenEditor(cfg.GetEditor(), cfg.Path())
			}

			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&editor, "editor", "e", "", "Set the editor to use for editing config")
	flags.StringVarP(&provider, "provider", "p", "", "Set the LLM provider: "+strings.Join(config.SupportedProviders(), ", "))
	flags.StringVarP(&model, "model", "m", "", "Set the LLM model")
	flags.Uint32Var(&maxDiffTokens, "max-diff-tokens", 0, fmt.Sprintf("Set the maximum number of diff tokens to include in the prompt, estimated at 2.5 characters a token; 0 disables the limit (default: %d)", config.DEFAULT_MAX_DIFF_TOKENS))
	flags.Uint32VarP(&maxDiffLines, "max-diff-lines", "d", 0, fmt.Sprintf("Set the maximum number of diff lines to include in the prompt (default: %d)", config.DEFAULT_MAX_DIFF_LINES))
	_ = flags.MarkDeprecated("max-diff-lines", "use --max-diff-tokens instead")
	flags.BoolVar(&contextEnrichment, "context-enrichment", false, "Enable or disable enclosing-declaration context for reviews (default: false)")
	flags.BoolVar(&structured, "structured-findings", false, "Enable or disable structured findings for reviews (default: false)")
	flags.BoolVar(&houseRules, "house-rules", true, "Enable or disable reading the repository's house rules (house_rules_files) into reviews (default: true)")
	flags.BoolVar(&checkFindings, "check-findings", true, "Enable or disable the second model call that checks structured findings against the code (default: true)")
	flags.BoolVar(&migrate, migrateFlag, false, "Move Bark's files from ~/.bark to the XDG directories (~/.config/bark, ~/.cache/bark and ~/.local/state/bark by default)")
	flags.StringVar(&minSeverity, "min-severity", "", "Set the lowest severity structured reviews show: "+strings.Join(config.Severities(), ", ")+` ("" shows every finding)`)
	flags.StringVar(&failOn, "fail-on", "", "Set the severity at or above which a plain review exits with status 3: "+strings.Join(config.Severities(), ", ")+` ("" never fails)`)

	flags.StringVar(&timeout, "timeout", "", `Set how long one model call may take before Bark gives up, such as 10m or 90s ("" uses each command's default)`)
	flags.StringVar(&effort, "effort", "", "Set how hard the model thinks before answering: "+strings.Join(config.Efforts(), ", ")+` ("" uses the model's default)`)

	// Old names, matching the config file keys. Kept so existing scripts keep
	// working; hidden from help, and pflag prints a deprecation notice.
	flags.StringVar(&provider, config.LLMProviderKey, "", "")
	flags.StringVar(&model, config.LLMModelKey, "", "")
	flags.Uint32Var(&maxDiffLines, config.MaxDiffLinesKey, 0, "")
	flags.BoolVar(&contextEnrichment, config.ContextEnrichmentKey, false, "")
	for oldName, newName := range map[string]string{
		config.LLMProviderKey:       "provider",
		config.LLMModelKey:          "model",
		config.MaxDiffLinesKey:      "max-diff-tokens",
		config.ContextEnrichmentKey: "context-enrichment",
	} {
		_ = flags.MarkDeprecated(oldName, "use --"+newName+" instead")
	}

	return cmd
}

// anyChanged reports whether any of the named flags was passed. Used where
// the zero value is a real setting, so it can't mean "not passed".
func anyChanged(flags *pflag.FlagSet, names ...string) bool {
	return slices.ContainsFunc(names, flags.Changed)
}

const migrateFlag = "migrate"

// migrateAlone returns an error if any flag other than --migrate is set:
// migrating doesn't load the config, so it can't apply settings, and a
// --config file has nothing to do with Bark's own directories.
func migrateAlone(flags *pflag.FlagSet) error {
	var others []string
	flags.Visit(func(f *pflag.Flag) {
		if f.Name != migrateFlag {
			others = append(others, "--"+f.Name)
		}
	})
	if len(others) > 0 {
		return fmt.Errorf("--%s can't be combined with %s", migrateFlag, strings.Join(others, ", "))
	}
	return nil
}

// migratingDirs reports whether cmd is `bark config --migrate`, which must
// run before (and instead of) loading the config: loading creates files.
func migratingDirs(cmd *cobra.Command) bool {
	if cmd.Name() != "config" {
		return false
	}
	migrate, err := cmd.Flags().GetBool(migrateFlag)
	return err == nil && migrate
}

// migrateDirs moves ~/.bark to the XDG directories and reports what it did
// to w.
func migrateDirs(env paths.Env, now time.Time, w io.Writer) error {
	result, err := env.Migrate(now)
	if errors.Is(err, paths.ErrNothingToMigrate) {
		_, _ = fmt.Fprintln(w, "Nothing to migrate: there is no ~/.bark directory.")
		return nil
	}
	if err != nil {
		return fmt.Errorf("migration failed, nothing was changed: %w", err)
	}

	for _, m := range result.Moved {
		_, _ = fmt.Fprintf(w, "Moved %s -> %s\n", m.From, m.To)
	}
	for _, name := range result.Skipped {
		_, _ = fmt.Fprintf(w, "Skipped %s: Bark doesn't use it\n", name)
	}
	_, _ = fmt.Fprintf(w, "Renamed %s to %s. Delete it once you've checked Bark works.\n", result.From, result.Backup)
	return nil
}

// initConfig sets up Bark's files in its config directory and loads the
// config file at path (the --config flag), or the default one when path is
// empty.
func initConfig(path string) (config.Config, error) {
	dirs, err := config.Paths()
	if err != nil {
		return nil, err
	}
	storage := dirs.Config

	cfg, err := loadConfigFrom(path)
	if err != nil {
		return nil, fmt.Errorf("error initializing config: %w", err)
	}

	if err := reviewers.Config(storage, false); err != nil {
		return nil, fmt.Errorf("error initializing reviewers: %w", err)
	}

	if err := instructions.Config(storage, false); err != nil {
		return nil, fmt.Errorf("error initializing instructions: %w", err)
	}

	if err := config.InitialiseCommitInstructions(); err != nil {
		return nil, fmt.Errorf("error initializing commit instructions: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if root, err := git.RepoRoot(ctx); err == nil {
		if err := applyRepoConfig(cfg, root); err != nil {
			return nil, err
		}
		if err := git.CheckIgnoreFile(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "bark: warning: %v\n(the other lines still apply)\n", err)
		}
	}

	for _, warning := range cfg.Deprecations() {
		fmt.Fprintf(os.Stderr, "bark: warning: %s\n", warning)
	}

	if dirs.Legacy {
		fmt.Fprintln(os.Stderr, legacyWarning(paths.System()))
	}

	return cfg, nil
}

// legacyWarning tells a user still on ~/.bark how to move to the XDG
// directories.
func legacyWarning(env paths.Env) string {
	target := "the XDG directories"
	if xdg, err := env.XDG(); err == nil {
		target = xdg.Config
	}
	return fmt.Sprintf("bark: warning: using ~/.bark; run 'bark config --migrate' to move Bark's files to %s", target)
}

// applyRepoConfig layers the config of the repository at root over cfg for
// this run, if the repository has one.
func applyRepoConfig(cfg config.Config, root string) error {
	path := config.RepoFilePath(root)
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	repo, err := config.ParseRepo(path, content)
	if err != nil {
		return err
	}
	cfg.ApplyRepo(path, repo)
	return nil
}

// loadConfigFrom loads the config file at path, or the default file (created
// if missing) when path is empty. A file passed with --config is read
// directly and never created, so a mistyped path fails instead of silently
// running with defaults.
func loadConfigFrom(path string) (config.Config, error) {
	if path == "" {
		defaultPath, err := config.DefaultFilePath()
		if err != nil {
			return nil, fmt.Errorf("error getting config path: %w", err)
		}
		return loadConfig(defaultPath)
	}

	data, err := readConfigData(path)
	if err != nil {
		return nil, err
	}

	return config.New(path, data), nil
}

// loadConfig reads the config file at path, first creating it with default
// values if it doesn't exist.
func loadConfig(path string) (config.Config, error) {
	created, err := config.EnsureFile(path)
	if err != nil {
		return nil, err
	}
	if created {
		fmt.Fprintln(os.Stderr, "Created config at", path)
	}

	data, err := readConfigData(path)
	if err != nil {
		return nil, err
	}

	return config.New(path, data), nil
}

// readConfigData reads the TOML config file at path. Keys missing from the
// file keep their default values.
func readConfigData(path string) (config.Data, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return config.Data{}, err
	}

	// Unmarshal only sets the keys present in the file.
	data := config.DefaultData()
	if err := toml.Unmarshal(contents, &data); err != nil {
		if decodeErr, ok := errors.AsType[*toml.DecodeError](err); ok {
			row, col := decodeErr.Position()
			return config.Data{}, fmt.Errorf("%s:%d:%d: %w", path, row, col, err)
		}
		return config.Data{}, fmt.Errorf("%s: %w", path, err)
	}

	if data.MinSeverity, err = config.NormaliseMinSeverity(data.MinSeverity); err != nil {
		return config.Data{}, fmt.Errorf("%s: %w", path, err)
	}
	if data.FailOn, err = config.NormaliseFailOn(data.FailOn); err != nil {
		return config.Data{}, fmt.Errorf("%s: %w", path, err)
	}
	if data.Effort, err = config.NormaliseEffort(data.Effort); err != nil {
		return config.Data{}, fmt.Errorf("%s: %w", path, err)
	}
	if data.LLMTimeout, err = config.NormaliseLLMTimeout(data.LLMTimeout); err != nil {
		return config.Data{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := config.ValidateHouseRulesFiles(data.HouseRulesFiles); err != nil {
		return config.Data{}, fmt.Errorf("%s: %w", path, err)
	}

	return data, nil
}
