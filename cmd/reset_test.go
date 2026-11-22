package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleReset(t *testing.T) {
	t.Run("needs a flag", func(t *testing.T) {
		isolateDirs(t)
		_, err := handleReset(withFlags(t, resetCmd()))
		assert.EqualError(t, err, "specify at least one of --reviewers or --instructions to reset")
	})

	t.Run("reviewers", func(t *testing.T) {
		storage := isolateDirs(t)
		require.NoError(t, reviewers.Config(storage, false))
		yoda, err := reviewers.GetPath(storage, "Yoda")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(yoda, []byte("edited"), 0o644))

		msg, err := handleReset(withFlags(t, resetCmd(), "-r"))
		require.NoError(t, err)
		assert.Equal(t, "Reviewers have been reset to default", msg)
		assert.NotEqual(t, "edited", readFile(t, yoda))
	})

	t.Run("instructions", func(t *testing.T) {
		isolateDirs(t)
		msg, err := handleReset(withFlags(t, resetCmd(), "-i"))
		require.NoError(t, err)
		assert.Equal(t, "Instructions have been reset to default", msg)
	})

	t.Run("hard reset removes custom files", func(t *testing.T) {
		storage := isolateDirs(t)
		custom := filepath.Join(storage, "reviewers", "Custom.md")
		require.NoError(t, os.MkdirAll(filepath.Dir(custom), 0o755))
		require.NoError(t, os.WriteFile(custom, []byte("mine"), 0o644))
		customInstr := filepath.Join(storage, "instructions", "Custom.md")
		require.NoError(t, os.MkdirAll(filepath.Dir(customInstr), 0o755))
		require.NoError(t, os.WriteFile(customInstr, []byte("mine"), 0o644))

		msg, err := handleReset(withFlags(t, resetCmd(), "-r", "-i", "--hard"))
		require.NoError(t, err)
		assert.Equal(t, "Reviewers and instructions have been reset to default", msg)
		_, err = os.Stat(custom)
		assert.ErrorIs(t, err, os.ErrNotExist)
		_, err = os.Stat(customInstr)
		assert.ErrorIs(t, err, os.ErrNotExist)
		_, err = reviewers.GetPath(storage, "Yoda")
		assert.NoError(t, err, "defaults are back")
	})

	t.Run("soft reset keeps custom files", func(t *testing.T) {
		storage := isolateDirs(t)
		custom := filepath.Join(storage, "reviewers", "Custom.md")
		require.NoError(t, os.MkdirAll(filepath.Dir(custom), 0o755))
		require.NoError(t, os.WriteFile(custom, []byte("mine"), 0o644))

		_, err := handleReset(withFlags(t, resetCmd(), "-r"))
		require.NoError(t, err)
		assert.Equal(t, "mine", readFile(t, custom))
	})
}
