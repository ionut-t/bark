package cmd

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// staticConfig returns a configFunc for commands built on their own, without
// the root command that loads the config.
func staticConfig(cfg config.Config) configFunc {
	return func() config.Config { return cfg }
}

// runRootCmd runs `bark <args>` with load as the config loader and returns
// the path load was called with.
func runRootCmd(t *testing.T, load configLoader, args ...string) (string, error) {
	t.Helper()

	var gotPath string
	cmd := newRootCmd(func(path string) (config.Config, error) {
		gotPath = path
		return load(path)
	})
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	return gotPath, cmd.Execute()
}

func TestRootCmd_ConfigFlagLoadsGivenFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args func(path string) []string
	}{
		{name: "before the subcommand", args: func(p string) []string { return []string{"--config", p, "config", "-m", "flag-model"} }},
		{name: "after the subcommand", args: func(p string) []string { return []string{"config", "--config", p, "-m", "flag-model"} }},
		{name: "with =", args: func(p string) []string { return []string{"config", "--config=" + p, "-m", "flag-model"} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "custom.toml")
			require.NoError(t, os.WriteFile(path, []byte("editor = 'true'\nllm_model = 'old-model'\n"), 0o644))

			gotPath, err := runRootCmd(t, loadConfigFrom, tt.args(path)...)
			require.NoError(t, err)

			assert.Equal(t, path, gotPath)
			assert.Equal(t, "flag-model", readSavedConfig(t, path).LLMModel)
		})
	}
}

func TestRootCmd_NoConfigFlagLoadsDefault(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "default.toml")
	require.NoError(t, os.WriteFile(path, []byte("editor = 'true'\n"), 0o644))

	// The real loader maps "" to $HOME/.bark/.config.toml; stand in for it
	// so the test doesn't touch HOME.
	gotPath, err := runRootCmd(t, func(p string) (config.Config, error) {
		require.Empty(t, p)
		return loadConfig(path)
	}, "config", "-m", "flag-model")
	require.NoError(t, err)

	assert.Empty(t, gotPath)
	assert.Equal(t, "flag-model", readSavedConfig(t, path).LLMModel)
}

func TestRootCmd_LoadErrorStopsTheCommand(t *testing.T) {
	t.Parallel()

	errLoad := errors.New("load failed")
	ran := false

	_, err := runRootCmd(t, func(string) (config.Config, error) {
		ran = true
		return nil, errLoad
	}, "config", "-m", "flag-model")

	assert.True(t, ran)
	assert.ErrorIs(t, err, errLoad)
}

func TestRootCmd_ConfigFlagIsInheritedBySubcommands(t *testing.T) {
	t.Parallel()

	root := newRootCmd(loadConfigFrom)
	require.NotEmpty(t, root.Commands())

	// Not parallel: cobra merges inherited flags into the shared command tree
	// on lookup, so concurrent lookups race.
	for _, sub := range root.Commands() {
		t.Run(sub.Name(), func(t *testing.T) {
			flag := sub.InheritedFlags().Lookup(configFlag)
			require.NotNil(t, flag, "--config must be accepted by %q", sub.Name())
		})
	}
}

func TestPlainFlag_OnlyOnCommandsThatReadIt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		command   string
		wantPlain bool
	}{
		{command: "review", wantPlain: true},
		{command: "commit", wantPlain: true},
		{command: "pr", wantPlain: true},
		// The root command opens the TUI and never reads --plain.
		{command: "", wantPlain: false},
		{command: "config", wantPlain: false},
		{command: "reset", wantPlain: false},
		{command: "add", wantPlain: false},
		{command: "delete", wantPlain: false},
		{command: "edit", wantPlain: false},
		{command: "actions", wantPlain: false},
		{command: "prices", wantPlain: false},
	}

	for _, tt := range tests {
		name := tt.command
		if name == "" {
			name = "root"
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			root := newRootCmd(loadConfig)
			cmd := root
			if tt.command != "" {
				var err error
				cmd, _, err = root.Find([]string{tt.command})
				require.NoError(t, err)
				require.Equal(t, tt.command, cmd.Name())
			}

			// ParseFlags merges inherited persistent flags, as Execute does.
			err := cmd.ParseFlags([]string{"--plain"})
			if !tt.wantPlain {
				require.ErrorContains(t, err, "unknown flag: --plain")
				assert.NotContains(t, cmd.UsageString(), "--plain")
				return
			}

			require.NoError(t, err)
			plain, err := cmd.Flags().GetBool(plainFlag)
			require.NoError(t, err)
			assert.True(t, plain)
			assert.Contains(t, cmd.UsageString(), "--plain")
		})
	}
}
