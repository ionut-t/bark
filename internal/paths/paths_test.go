package paths

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testEnv is an Env with home as the home directory, vars as the
// environment and goos as the OS.
func testEnv(home, goos string, vars map[string]string) Env {
	return Env{
		Getenv: func(key string) string { return vars[key] },
		Home:   func() (string, error) { return home, nil },
		GOOS:   goos,
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestResolve_DefaultsWithoutXDGVars(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	for _, goos := range []string{"linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			got, err := testEnv(home, goos, nil).Resolve()
			require.NoError(t, err)

			assert.Equal(t, Paths{
				Config: filepath.Join(home, ".config", "bark"),
				Cache:  filepath.Join(home, ".cache", "bark"),
				State:  filepath.Join(home, ".local", "state", "bark"),
			}, got)
		})
	}
}

func TestResolve_UsesXDGVars(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	got, err := testEnv(home, "darwin", map[string]string{
		"XDG_CONFIG_HOME": "/x/config",
		"XDG_CACHE_HOME":  "/x/cache",
		"XDG_STATE_HOME":  "/x/state",
	}).Resolve()
	require.NoError(t, err)

	assert.Equal(t, Paths{
		Config: filepath.Join("/x/config", "bark"),
		Cache:  filepath.Join("/x/cache", "bark"),
		State:  filepath.Join("/x/state", "bark"),
	}, got)
}

func TestResolve_IgnoresRelativeXDGVars(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	got, err := testEnv(home, "linux", map[string]string{
		"XDG_CONFIG_HOME": "relative/config",
		"XDG_CACHE_HOME":  "./cache",
		"XDG_STATE_HOME":  "state",
	}).Resolve()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(home, ".config", "bark"), got.Config)
	assert.Equal(t, filepath.Join(home, ".cache", "bark"), got.Cache)
	assert.Equal(t, filepath.Join(home, ".local", "state", "bark"), got.State)
}

func TestResolve_Windows(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	appData := filepath.Join(home, "AppData", "Roaming")
	localAppData := filepath.Join(home, "AppData", "Local")

	got, err := testEnv(home, "windows", map[string]string{
		"AppData":      appData,
		"LocalAppData": localAppData,
	}).Resolve()
	require.NoError(t, err)

	assert.Equal(t, Paths{
		Config: filepath.Join(appData, "bark"),
		Cache:  filepath.Join(localAppData, "bark"),
		State:  filepath.Join(localAppData, "bark"),
	}, got)
}

func TestResolve_WindowsXDGVarsWin(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	config := filepath.Join(home, "xdg-config")

	got, err := testEnv(home, "windows", map[string]string{
		"AppData":         filepath.Join(home, "AppData", "Roaming"),
		"XDG_CONFIG_HOME": config,
	}).Resolve()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(config, "bark"), got.Config)
}

func TestResolve_LegacyDirWhenOnlyItHasAConfig(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".bark", ".config.toml"), "")

	got, err := testEnv(home, "linux", nil).Resolve()
	require.NoError(t, err)

	legacy := filepath.Join(home, ".bark")
	assert.Equal(t, Paths{Config: legacy, Cache: legacy, State: legacy, Legacy: true}, got)
	assert.Equal(t, filepath.Join(legacy, ".config.toml"), got.ConfigFile())
}

func TestResolve_NewLayoutWinsOverLegacy(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".bark", ".config.toml"), "")
	writeFile(t, filepath.Join(home, ".config", "bark", "config.toml"), "")

	got, err := testEnv(home, "linux", nil).Resolve()
	require.NoError(t, err)

	assert.False(t, got.Legacy)
	assert.Equal(t, filepath.Join(home, ".config", "bark", "config.toml"), got.ConfigFile())
}

func TestResolve_LegacyDirWithoutConfigIsIgnored(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".bark", "prices.json"), "{}")

	got, err := testEnv(home, "linux", nil).Resolve()
	require.NoError(t, err)

	assert.False(t, got.Legacy)
}

func TestResolve_HomeError(t *testing.T) {
	t.Parallel()

	errHome := errors.New("no home")
	env := Env{
		Getenv: func(string) string { return "" },
		Home:   func() (string, error) { return "", errHome },
		GOOS:   "linux",
	}

	_, err := env.Resolve()
	assert.ErrorIs(t, err, errHome)
}

func TestSystemUsesTheProcessEnvironment(t *testing.T) {
	t.Setenv("BARK_PATHS_TEST", "value")

	env := System()
	assert.Equal(t, runtime.GOOS, env.GOOS)
	assert.Equal(t, "value", env.Getenv("BARK_PATHS_TEST"))

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	got, err := env.Home()
	require.NoError(t, err)
	assert.Equal(t, home, got)
}

func TestResolveUsesTheProcessEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG defaults differ on Windows")
	}
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))

	got, err := Resolve()
	require.NoError(t, err)
	assert.Equal(t, Paths{
		Config: filepath.Join(root, "config", appName),
		Cache:  filepath.Join(root, "cache", appName),
		State:  filepath.Join(root, "state", appName),
	}, got)
}
