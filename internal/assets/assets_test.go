package assets

import (
	"embed"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/prompts
var testFS embed.FS

const dir = "things"

func configure(t *testing.T, storage string, reset bool) {
	t.Helper()
	require.NoError(t, Config(ConfigOptions{
		Storage:  storage,
		Reset:    reset,
		AssetDir: dir,
		FromDir:  "testdata/prompts",
		EmbedFS:  testFS,
	}))
}

// fakeEditor returns an "editor" that writes content into the file it is
// given.
func fakeEditor(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "editor.sh")
	script := "#!/bin/sh\nprintf '%s' '" + content + "' > \"$1\"\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

func TestConfigUnpacksTopLevelFilesOnly(t *testing.T) {
	storage := t.TempDir()
	configure(t, storage, false)

	list, err := GetAssets(storage, dir)
	require.NoError(t, err)
	assert.Equal(t, []Asset{
		{Name: "Alpha", Prompt: "Alpha prompt.\n"},
		{Name: "Beta", Prompt: "Beta prompt.\n"},
	}, list)

	_, err = os.Stat(filepath.Join(storage, dir, "nested"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestConfigLeavesExistingDirUnlessReset(t *testing.T) {
	storage := t.TempDir()
	configure(t, storage, false)

	alpha := filepath.Join(storage, dir, "Alpha.md")
	require.NoError(t, os.WriteFile(alpha, []byte("edited"), 0o644))
	require.NoError(t, os.Remove(filepath.Join(storage, dir, "Beta.md")))

	configure(t, storage, false)
	content, err := os.ReadFile(alpha)
	require.NoError(t, err)
	assert.Equal(t, "edited", string(content))
	_, err = os.Stat(filepath.Join(storage, dir, "Beta.md"))
	assert.ErrorIs(t, err, os.ErrNotExist, "an existing directory is not refilled")

	configure(t, storage, true)
	content, err = os.ReadFile(alpha)
	require.NoError(t, err)
	assert.Equal(t, "Alpha prompt.\n", string(content))
	_, err = os.Stat(filepath.Join(storage, dir, "Beta.md"))
	assert.NoError(t, err)
}

func TestConfigMissingSourceDir(t *testing.T) {
	err := Config(ConfigOptions{
		Storage:  t.TempDir(),
		AssetDir: dir,
		FromDir:  "testdata/missing",
		EmbedFS:  testFS,
	})
	assert.Error(t, err)
}

func TestGetAssetsSkipsDirectories(t *testing.T) {
	storage := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(storage, dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(storage, dir, "One.md"), []byte("one"), 0o644))

	list, err := GetAssets(storage, dir)
	require.NoError(t, err)
	assert.Equal(t, []Asset{{Name: "One", Prompt: "one"}}, list)
}

func TestGetAssetsMissingDir(t *testing.T) {
	list, err := GetAssets(t.TempDir(), dir)
	assert.Error(t, err)
	assert.Empty(t, list)
}

func TestGetPathAndDelete(t *testing.T) {
	storage := t.TempDir()
	configure(t, storage, false)

	path, err := GetPath(storage, dir, "Alpha")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(storage, dir, "Alpha.md"), path)

	_, err = GetPath(storage, dir, "Gamma")
	assert.ErrorIs(t, err, os.ErrNotExist)

	require.NoError(t, Delete(storage, dir, "Alpha"))
	_, err = GetPath(storage, dir, "Alpha")
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.ErrorIs(t, Delete(storage, dir, "Alpha"), os.ErrNotExist)
}

func TestRemoveAssetDir(t *testing.T) {
	storage := t.TempDir()
	configure(t, storage, false)

	require.NoError(t, RemoveAssetDir(storage, dir))
	_, err := os.Stat(filepath.Join(storage, dir))
	assert.ErrorIs(t, err, os.ErrNotExist)

	assert.NoError(t, RemoveAssetDir(storage, dir), "removing a missing dir is not an error")
}

func TestAdd(t *testing.T) {
	t.Run("saves what the editor wrote", func(t *testing.T) {
		storage := t.TempDir()
		require.NoError(t, Add(storage, dir, "New", fakeEditor(t, "New prompt.")))

		content, err := os.ReadFile(filepath.Join(storage, dir, "New.md"))
		require.NoError(t, err)
		assert.Equal(t, "New prompt.", string(content))
	})

	t.Run("refuses an existing name", func(t *testing.T) {
		storage := t.TempDir()
		configure(t, storage, false)
		err := Add(storage, dir, "Alpha", fakeEditor(t, "x"))
		assert.ErrorIs(t, err, os.ErrExist)
	})

	t.Run("refuses empty content", func(t *testing.T) {
		storage := t.TempDir()
		err := Add(storage, dir, "Empty", fakeEditor(t, ""))
		assert.EqualError(t, err, "content cannot be empty")
		_, statErr := os.Stat(filepath.Join(storage, dir, "Empty.md"))
		assert.ErrorIs(t, statErr, os.ErrNotExist)
	})

	t.Run("reports a missing editor", func(t *testing.T) {
		err := Add(t.TempDir(), dir, "X", "")
		assert.ErrorContains(t, err, "editor is not configured")
	})

	t.Run("reports a failing editor", func(t *testing.T) {
		err := Add(t.TempDir(), dir, "X", "false")
		assert.ErrorContains(t, err, "error opening editor")
	})
}
