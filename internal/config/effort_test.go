package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEffort(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), configFileName)
	cfg := New(path, Data{Editor: "vi"})
	assert.Empty(t, cfg.GetEffort(), "the provider's default unless set")

	require.NoError(t, cfg.SetEffort("Medium"))
	assert.Equal(t, "medium", cfg.GetEffort())
	assert.Equal(t, "medium", readSaved(t, path).Effort)

	assert.Error(t, cfg.SetEffort("ludicrous"))
	assert.Error(t, cfg.OverrideEffort("ludicrous"))
	assert.Equal(t, "medium", cfg.GetEffort())

	cfg.ApplyRepo("/repo/.bark/config.toml", RepoData{Effort: new("high")})
	assert.Equal(t, "high", cfg.GetEffort(), "the repository's value beats the user's")

	require.NoError(t, cfg.OverrideEffort("LOW"))
	assert.Equal(t, "low", cfg.GetEffort(), "the flag beats the repository's value")
	assert.Equal(t, "medium", readSaved(t, path).Effort, "overrides are never saved")

	require.NoError(t, cfg.SetEffort(""))
	assert.Equal(t, "high", cfg.GetEffort(), "setting replaces the override")
}

func TestParseRepo_Effort(t *testing.T) {
	t.Parallel()

	got, err := ParseRepo("/repo/.bark/config.toml", []byte("effort = 'Low'\n"))
	require.NoError(t, err)
	assert.Equal(t, RepoData{Effort: new("low")}, got)

	_, err = ParseRepo("/repo/.bark/config.toml", []byte("effort = 'ludicrous'\n"))
	require.Error(t, err)
	for _, s := range []string{"config.toml", "ludicrous", "low, medium, high"} {
		assert.Contains(t, err.Error(), s)
	}
}
