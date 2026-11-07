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
// that starts with the given contents, and returns what was saved. The config
// package reads global viper (R1), so these tests can't run in parallel.
func runConfigCmd(t *testing.T, existing string, args ...string) savedConfig {
	t.Helper()

	// If no flag counts as set, the command opens the editor instead; make
	// that a no-op so the test fails on the saved values rather than hanging.
	t.Setenv("EDITOR", "true")

	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(existing), 0o644))
	viper.SetConfigFile(path)
	require.NoError(t, viper.ReadInConfig())
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
			assert.Equal(t, want, runConfigCmd(t, "", tt.args...))
		})
	}
}

// Zero and false are real settings (0 disables the diff limit), so passing
// them must save them rather than be treated as "flag not given".
func TestConfigCmd_SetsZeroValues(t *testing.T) {
	const existing = "max_diff_lines = 300\ncontext_enrichment = true\n"

	tests := []struct {
		name string
		args []string
		want savedConfig
	}{
		{name: "-d 0", args: []string{"-d", "0"}, want: savedConfig{MaxDiffLines: 0, ContextEnrichment: true}},
		{name: "--max-diff-lines 0", args: []string{"--max-diff-lines", "0"}, want: savedConfig{MaxDiffLines: 0, ContextEnrichment: true}},
		{name: "--max_diff_lines 0", args: []string{"--max_diff_lines", "0"}, want: savedConfig{MaxDiffLines: 0, ContextEnrichment: true}},
		{name: "--context-enrichment=false", args: []string{"--context-enrichment=false"}, want: savedConfig{MaxDiffLines: 300, ContextEnrichment: false}},
		{name: "--context_enrichment=false", args: []string{"--context_enrichment=false"}, want: savedConfig{MaxDiffLines: 300, ContextEnrichment: false}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runConfigCmd(t, existing, tt.args...)
			assert.Equal(t, tt.want.MaxDiffLines, got.MaxDiffLines)
			assert.Equal(t, tt.want.ContextEnrichment, got.ContextEnrichment)
		})
	}
}

// Setting one value must keep the others already in the file.
func TestConfigCmd_KeepsOtherValues(t *testing.T) {
	const existing = "llm_provider = 'openai'\nllm_model = 'gpt-x'\nmax_diff_lines = 300\ncontext_enrichment = true\n"

	got := runConfigCmd(t, existing, "-e", "nano")
	assert.Equal(t, savedConfig{LLMProvider: "openai", LLMModel: "gpt-x", MaxDiffLines: 300, ContextEnrichment: true}, got)
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
