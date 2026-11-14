package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPRDataParamsCarryInstructions(t *testing.T) {
	m := &Model{
		config:            config.New("", config.Data{}),
		instructionValues: []string{"Be brief.", "Mention migrations."},
	}

	assert.Equal(t, []string{"Be brief.", "Mention migrations."}, m.prDataParams().instructions)
}

func TestResolvePRInstructions(t *testing.T) {
	file := filepath.Join(t.TempDir(), "pr.md")
	require.NoError(t, os.WriteFile(file, []byte("From a file.\n"), 0o644))

	t.Run("flag values replace the fallback", func(t *testing.T) {
		got, err := resolvePRInstructions([]string{file, "Be brief."}, "fallback")
		require.NoError(t, err)
		assert.Equal(t, "From a file.\n\nBe brief.", got)
	})

	t.Run("no values uses the fallback", func(t *testing.T) {
		t.Chdir(t.TempDir()) // no .bark/pr.md here
		got, err := resolvePRInstructions(nil, "fallback")
		require.NoError(t, err)
		assert.Equal(t, "fallback", got)
	})
}
