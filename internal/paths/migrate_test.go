package paths

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var migrateTime = time.Date(2026, 11, 17, 10, 30, 0, 0, time.UTC)

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

// legacyHome returns a home directory with a full ~/.bark in it.
func legacyHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	legacy := filepath.Join(home, ".bark")
	writeFile(t, filepath.Join(legacy, ".config.toml"), "editor = 'nvim'\n")
	writeFile(t, filepath.Join(legacy, "commit.md"), "commit")
	writeFile(t, filepath.Join(legacy, "pull_request_description.md"), "pr")
	writeFile(t, filepath.Join(legacy, "reviewers", "linus.md"), "linus")
	writeFile(t, filepath.Join(legacy, "instructions", "nested", "go.md"), "go")
	writeFile(t, filepath.Join(legacy, "prices.json"), "{}")
	writeFile(t, filepath.Join(legacy, "logs", "a-review.log"), "log")
	writeFile(t, filepath.Join(legacy, "pr.md"), "stray")
	return home
}

func TestMigrate_MovesEverythingToItsXDGDir(t *testing.T) {
	t.Parallel()

	home := legacyHome(t)
	env := testEnv(home, "linux", nil)

	result, err := env.Migrate(migrateTime)
	require.NoError(t, err)

	config := filepath.Join(home, ".config", "bark")
	cache := filepath.Join(home, ".cache", "bark")
	state := filepath.Join(home, ".local", "state", "bark")

	assert.Equal(t, "editor = 'nvim'\n", readFile(t, filepath.Join(config, "config.toml")))
	assert.Equal(t, "commit", readFile(t, filepath.Join(config, "commit.md")))
	assert.Equal(t, "pr", readFile(t, filepath.Join(config, "pull_request_description.md")))
	assert.Equal(t, "linus", readFile(t, filepath.Join(config, "reviewers", "linus.md")))
	assert.Equal(t, "go", readFile(t, filepath.Join(config, "instructions", "nested", "go.md")))
	assert.Equal(t, "{}", readFile(t, filepath.Join(cache, "prices.json")))
	assert.Equal(t, "log", readFile(t, filepath.Join(state, "logs", "a-review.log")))
	assert.NoFileExists(t, filepath.Join(config, ".config.toml"))
	assert.NoFileExists(t, filepath.Join(config, "pr.md"))

	backup := filepath.Join(home, ".bark.bak")
	assert.NoDirExists(t, filepath.Join(home, ".bark"))
	assert.Equal(t, "stray", readFile(t, filepath.Join(backup, "pr.md")))

	assert.Equal(t, filepath.Join(home, ".bark"), result.From)
	assert.Equal(t, backup, result.Backup)
	assert.Equal(t, []string{"pr.md"}, result.Skipped)
	assert.Len(t, result.Moved, 7)
	assert.Contains(t, result.Moved, Move{
		From: filepath.Join(home, ".bark", ".config.toml"),
		To:   filepath.Join(config, "config.toml"),
	})

	got, err := env.Resolve()
	require.NoError(t, err)
	assert.False(t, got.Legacy)
}

func TestMigrate_PartialLegacyDir(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".bark", ".config.toml"), "x")

	result, err := testEnv(home, "linux", nil).Migrate(migrateTime)
	require.NoError(t, err)

	assert.Equal(t, "x", readFile(t, filepath.Join(home, ".config", "bark", "config.toml")))
	assert.Len(t, result.Moved, 1)
	assert.Empty(t, result.Skipped)
	assert.NoDirExists(t, filepath.Join(home, ".cache", "bark"))
	assert.NoDirExists(t, filepath.Join(home, ".local", "state", "bark"))
}

func TestMigrate_TimestampsTheBackupWhenOneExists(t *testing.T) {
	t.Parallel()

	home := legacyHome(t)
	writeFile(t, filepath.Join(home, ".bark.bak", "old"), "old")

	result, err := testEnv(home, "linux", nil).Migrate(migrateTime)
	require.NoError(t, err)

	want := filepath.Join(home, ".bark.bak-20261117-103000")
	assert.Equal(t, want, result.Backup)
	assert.DirExists(t, want)
	assert.Equal(t, "old", readFile(t, filepath.Join(home, ".bark.bak", "old")))
}

func TestMigrate_NothingToMigrate(t *testing.T) {
	t.Parallel()

	home := t.TempDir()

	_, err := testEnv(home, "linux", nil).Migrate(migrateTime)
	assert.ErrorIs(t, err, ErrNothingToMigrate)
}

func TestMigrate_RunTwice(t *testing.T) {
	t.Parallel()

	home := legacyHome(t)
	env := testEnv(home, "linux", nil)

	_, err := env.Migrate(migrateTime)
	require.NoError(t, err)

	_, err = env.Migrate(migrateTime)
	assert.ErrorIs(t, err, ErrNothingToMigrate)
}

func TestMigrate_RefusesWhenADestinationExists(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"config":    filepath.Join(".config", "bark", "config.toml"),
		"reviewers": filepath.Join(".config", "bark", "reviewers"),
		"prices":    filepath.Join(".cache", "bark", "prices.json"),
	}

	for name, rel := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			home := legacyHome(t)
			existing := filepath.Join(home, rel)
			writeFile(t, existing, "mine")

			_, err := testEnv(home, "linux", nil).Migrate(migrateTime)
			require.Error(t, err)
			assert.Contains(t, err.Error(), existing)

			// Nothing moved and nothing overwritten.
			assert.DirExists(t, filepath.Join(home, ".bark"))
			assert.Equal(t, "mine", readFile(t, existing))
			assert.NoFileExists(t, filepath.Join(home, ".config", "bark", "commit.md"))
		})
	}
}

func TestMigrate_RollsBackWhenACopyFails(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("root can read unreadable files")
	}

	home := legacyHome(t)
	unreadable := filepath.Join(home, ".bark", "reviewers", "secret.md")
	writeFile(t, unreadable, "x")
	require.NoError(t, os.Chmod(unreadable, 0o000))
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o644) })

	_, err := testEnv(home, "linux", nil).Migrate(migrateTime)
	require.Error(t, err)

	assert.DirExists(t, filepath.Join(home, ".bark"))
	assert.NoDirExists(t, filepath.Join(home, ".config", "bark"))
	assert.NoDirExists(t, filepath.Join(home, ".cache", "bark"))
	assert.NoDirExists(t, filepath.Join(home, ".local", "state", "bark"))
	assert.Equal(t, "commit", readFile(t, filepath.Join(home, ".bark", "commit.md")))
}

func TestMigrate_RollbackKeepsDirsThatAlreadyExisted(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("root can read unreadable files")
	}

	home := legacyHome(t)
	other := filepath.Join(home, ".config", "bark", "notes.txt")
	writeFile(t, other, "keep")

	unreadable := filepath.Join(home, ".bark", "logs", "secret.log")
	writeFile(t, unreadable, "x")
	require.NoError(t, os.Chmod(unreadable, 0o000))
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o644) })

	_, err := testEnv(home, "linux", nil).Migrate(migrateTime)
	require.Error(t, err)

	assert.Equal(t, "keep", readFile(t, other))
	assert.NoFileExists(t, filepath.Join(home, ".config", "bark", "config.toml"))
}
