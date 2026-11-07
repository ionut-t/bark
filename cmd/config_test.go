package cmd

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type savedConfig struct {
	LLMProvider       string `toml:"llm_provider"`
	LLMModel          string `toml:"llm_model"`
	MaxDiffLines      uint32 `toml:"max_diff_lines"`
	ContextEnrichment bool   `toml:"context_enrichment"`
}

// runConfigCmd runs `bark config <args>` against a config file in a temp dir
// and returns what was saved. The config package reads global viper (R1), so
// these tests can't run in parallel.
func runConfigCmd(t *testing.T, args ...string) savedConfig {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.toml")
	viper.SetConfigFile(path)
	t.Cleanup(viper.Reset)

	cmd := configCmd()
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	require.NoError(t, cmd.Execute())

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var saved savedConfig
	require.NoError(t, toml.Unmarshal(data, &saved))
	return saved
}

func TestConfigCmd_SetsValues(t *testing.T) {
	want := savedConfig{
		LLMProvider:       "anthropic",
		LLMModel:          "claude-sonnet-5-5",
		MaxDiffLines:      500,
		ContextEnrichment: true,
	}

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "long flags",
			args: []string{"--provider", "anthropic", "--model", "claude-sonnet-5-5", "--max-diff-lines", "500", "--context-enrichment"},
		},
		{
			name: "short flags",
			args: []string{"-p", "anthropic", "-m", "claude-sonnet-5-5", "-d", "500", "--context-enrichment"},
		},
		{
			name: "deprecated flags still work",
			args: []string{"--llm_provider", "anthropic", "--llm_model", "claude-sonnet-5-5", "--max_diff_lines", "500", "--context_enrichment"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, want, runConfigCmd(t, tt.args...))
		})
	}
}

func TestConfigCmd_DisablesContextEnrichment(t *testing.T) {
	for _, flag := range []string{"--context-enrichment=false", "--context_enrichment=false"} {
		t.Run(flag, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			require.NoError(t, os.WriteFile(path, []byte("context_enrichment = true\n"), 0o644))
			viper.SetConfigFile(path)
			require.NoError(t, viper.ReadInConfig())
			t.Cleanup(viper.Reset)

			cmd := configCmd()
			cmd.SetArgs([]string{flag})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			require.NoError(t, cmd.Execute())

			data, err := os.ReadFile(path)
			require.NoError(t, err)
			var saved savedConfig
			require.NoError(t, toml.Unmarshal(data, &saved))
			assert.False(t, saved.ContextEnrichment)
		})
	}
}

func TestConfigCmd_OldFlagNamesAreHiddenAndDeprecated(t *testing.T) {
	t.Parallel()

	renamed := map[string]string{
		config.LLMProviderKey:       "provider",
		config.LLMModelKey:          "model",
		config.MaxDiffLinesKey:      "max-diff-lines",
		config.ContextEnrichmentKey: "context-enrichment",
	}

	flags := configCmd().Flags()
	for oldName, newName := range renamed {
		old := flags.Lookup(oldName)
		require.NotNil(t, old, oldName)
		assert.True(t, old.Hidden, "%s should be hidden", oldName)
		assert.Contains(t, old.Deprecated, "--"+newName)
		assert.Empty(t, old.Shorthand, "%s should not keep a shorthand", oldName)

		require.NotNil(t, flags.Lookup(newName), newName)
	}
}
