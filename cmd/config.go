package cmd

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/instructions"
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
		maxDiffLines      uint32
		contextEnrichment bool
		structured        bool
	)

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
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
	flags.Uint32VarP(&maxDiffLines, "max-diff-lines", "d", 0, fmt.Sprintf("Set the maximum number of diff lines to include in the prompt (default: %d)", config.DEFAULT_MAX_DIFF_LINES))
	flags.BoolVar(&contextEnrichment, "context-enrichment", false, "Enable or disable enclosing-declaration context for reviews (default: false)")
	flags.BoolVar(&structured, "structured-findings", false, "Enable or disable structured findings for reviews (default: false)")

	// Old names, matching the config file keys. Kept so existing scripts keep
	// working; hidden from help, and pflag prints a deprecation notice.
	flags.StringVar(&provider, config.LLMProviderKey, "", "")
	flags.StringVar(&model, config.LLMModelKey, "", "")
	flags.Uint32Var(&maxDiffLines, config.MaxDiffLinesKey, 0, "")
	flags.BoolVar(&contextEnrichment, config.ContextEnrichmentKey, false, "")
	for oldName, newName := range map[string]string{
		config.LLMProviderKey:       "provider",
		config.LLMModelKey:          "model",
		config.MaxDiffLinesKey:      "max-diff-lines",
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

// initConfig sets up Bark's files in $HOME/.bark and loads the config file
// at path (the --config flag), or the default one when path is empty.
func initConfig(path string) (config.Config, error) {
	cfg, err := loadConfigFrom(path)
	if err != nil {
		return nil, fmt.Errorf("error initializing config: %w", err)
	}

	storage, err := config.GetStorage()
	if err != nil {
		return nil, fmt.Errorf("error getting storage path: %w", err)
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

	return cfg, nil
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
		var decodeErr *toml.DecodeError
		if errors.As(err, &decodeErr) {
			row, col := decodeErr.Position()
			return config.Data{}, fmt.Errorf("%s:%d:%d: %w", path, row, col, err)
		}
		return config.Data{}, fmt.Errorf("%s: %w", path, err)
	}

	return data, nil
}
