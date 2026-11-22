package reviewers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigUnpacksEmbeddedReviewers(t *testing.T) {
	storage := t.TempDir()
	require.NoError(t, Config(storage, false))

	list, err := Get(storage)
	require.NoError(t, err)
	require.NotEmpty(t, list)

	names := make([]string, 0, len(list))
	for _, r := range list {
		names = append(names, r.Name)
		assert.NotEmpty(t, r.Prompt, "reviewer %q has an empty prompt", r.Name)
	}
	assert.Contains(t, names, "Linus Torvalds")
}

func TestConfigKeepsUserEditsUnlessReset(t *testing.T) {
	storage := t.TempDir()
	require.NoError(t, Config(storage, false))

	path, err := GetPath(storage, "Yoda")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("edited"), 0o644))

	require.NoError(t, Config(storage, false))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "edited", string(content))

	require.NoError(t, Config(storage, true))
	content, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.NotEqual(t, "edited", string(content))
}

func TestGetMissingStorage(t *testing.T) {
	_, err := Get(filepath.Join(t.TempDir(), "missing"))
	assert.Error(t, err)
}

func TestFind(t *testing.T) {
	list := []Reviewer{
		{Name: "Linus Torvalds", Prompt: "linus"},
		{Name: "Rob Pike", Prompt: "rob"},
	}

	got, err := Find("pike", list)
	require.NoError(t, err)
	assert.Equal(t, "rob", got.Prompt)

	got, err = Find("LINUS", list)
	require.NoError(t, err)
	assert.Equal(t, "Linus Torvalds", got.Name)

	_, err = Find("yoda", list)
	assert.EqualError(t, err, "reviewer 'yoda' not found")

	_, err = Find("anyone", nil)
	assert.Error(t, err)
}

func TestFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Grumpy Dev.md")
	require.NoError(t, os.WriteFile(path, []byte("Be grumpy."), 0o644))

	got, err := FromFile(path)
	require.NoError(t, err)
	assert.Equal(t, &Reviewer{Name: "Grumpy Dev", Prompt: "Be grumpy."}, got)

	_, err = FromFile(filepath.Join(t.TempDir(), "missing.md"))
	assert.ErrorContains(t, err, "error reading reviewer file")
}

func TestGetEmbedded(t *testing.T) {
	got, err := GetEmbedded("Linus Torvalds")
	require.NoError(t, err)
	assert.Equal(t, "Linus Torvalds", got.Name)
	assert.NotEmpty(t, got.Prompt)

	_, err = GetEmbedded("Nobody")
	assert.EqualError(t, err, "reviewer 'Nobody' not found")
}

func TestDeleteAndRemoveDir(t *testing.T) {
	storage := t.TempDir()
	require.NoError(t, Config(storage, false))

	require.NoError(t, Delete(storage, "Yoda"))
	_, err := GetPath(storage, "Yoda")
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Error(t, Delete(storage, "Yoda"))

	require.NoError(t, RemoveDir(storage))
	_, err = os.Stat(filepath.Join(storage, assetDirName))
	assert.ErrorIs(t, err, os.ErrNotExist)
}
