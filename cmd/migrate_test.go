package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolateHome points HOME at a temp dir and clears the XDG variables, so
// Bark's directories are under it. Tests using it can't run in parallel.
func isolateHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME"} {
		t.Setenv(key, "")
	}
	return home
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// runMigrateCmd runs `bark <args>` with a loader that fails the test if it
// is called: --migrate must not load (and so create) any config.
func runMigrateCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()

	cmd := newRootCmd(func(string) (config.Config, error) {
		t.Error("config was loaded")
		return nil, os.ErrInvalid
	})
	var out bytes.Buffer
	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	return out.String(), err
}

func TestConfigMigrate_MovesLegacyDir(t *testing.T) {
	home := isolateHome(t)
	writeTestFile(t, filepath.Join(home, ".bark", ".config.toml"), "editor = 'nvim'\n")
	writeTestFile(t, filepath.Join(home, ".bark", "prices.json"), "{}")
	writeTestFile(t, filepath.Join(home, ".bark", "logs", "x.log"), "log")
	writeTestFile(t, filepath.Join(home, ".bark", "pr.md"), "stray")

	out, err := runMigrateCmd(t, "config", "--migrate")
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(home, ".config", "bark", "config.toml"))
	assert.FileExists(t, filepath.Join(home, ".cache", "bark", "prices.json"))
	assert.FileExists(t, filepath.Join(home, ".local", "state", "bark", "logs", "x.log"))
	assert.DirExists(t, filepath.Join(home, ".bark.bak"))

	assert.Contains(t, out, filepath.Join(home, ".config", "bark", "config.toml"))
	assert.Contains(t, out, "pr.md")
	assert.Contains(t, out, filepath.Join(home, ".bark.bak"))
}

func TestConfigMigrate_NothingToMigrate(t *testing.T) {
	isolateHome(t)

	out, err := runMigrateCmd(t, "config", "--migrate")
	require.NoError(t, err)
	assert.Contains(t, out, "Nothing to migrate")
}

func TestConfigMigrate_RefusesWhenNewConfigExists(t *testing.T) {
	home := isolateHome(t)
	writeTestFile(t, filepath.Join(home, ".bark", ".config.toml"), "")
	existing := filepath.Join(home, ".config", "bark", "config.toml")
	writeTestFile(t, existing, "")

	_, err := runMigrateCmd(t, "config", "--migrate")
	require.Error(t, err)
	assert.Contains(t, err.Error(), existing)
	assert.DirExists(t, filepath.Join(home, ".bark"))
}

func TestConfigMigrate_RejectsOtherFlags(t *testing.T) {
	tests := map[string][]string{
		"setter":  {"config", "--migrate", "--model", "m"},
		"config":  {"config", "--migrate", "--config", "x.toml"},
		"no-bool": {"config", "--migrate", "--structured-findings=false"},
	}

	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			home := isolateHome(t)
			writeTestFile(t, filepath.Join(home, ".bark", ".config.toml"), "")

			_, err := runMigrateCmd(t, args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "--migrate")
			assert.DirExists(t, filepath.Join(home, ".bark"))
		})
	}
}

func TestInitConfig_LegacyDirKeepsWorkingAndWarns(t *testing.T) {
	home := isolateHome(t)
	writeTestFile(t, filepath.Join(home, ".bark", ".config.toml"), "llm_model = 'legacy-model'\n")

	stderr := captureStderr(t, func() {
		cfg, err := initConfig("")
		require.NoError(t, err)

		model, _ := cfg.GetLLMModel()
		assert.Equal(t, "legacy-model", model)
	})

	assert.Contains(t, stderr, "bark config --migrate")
	// Nothing is created in the new location while ~/.bark is in use.
	assert.NoDirExists(t, filepath.Join(home, ".config", "bark"))
	assert.DirExists(t, filepath.Join(home, ".bark", "reviewers"))
}

func TestInitConfig_FreshInstallUsesXDG(t *testing.T) {
	home := isolateHome(t)

	stderr := captureStderr(t, func() {
		_, err := initConfig("")
		require.NoError(t, err)
	})

	assert.NotContains(t, stderr, "--migrate")
	assert.FileExists(t, filepath.Join(home, ".config", "bark", "config.toml"))
	assert.FileExists(t, filepath.Join(home, ".config", "bark", "commit.md"))
	assert.DirExists(t, filepath.Join(home, ".config", "bark", "reviewers"))
	assert.NoDirExists(t, filepath.Join(home, ".bark"))
}

// captureStderr returns what fn writes to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()

	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)
	return buf.String()
}

func TestConfigMigrate_FalseLoadsTheConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("editor = 'true'\n"), 0o644))

	_, err := runRootCmd(t, func(string) (config.Config, error) {
		return loadConfig(path)
	}, "config", "--migrate=false", "-m", "flag-model")
	require.NoError(t, err)
	assert.Equal(t, "flag-model", readSavedConfig(t, path).LLMModel)
}
