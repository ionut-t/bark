package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleAddCmd(t *testing.T) {
	storage := isolateDirs(t)
	cfg := config.New("", config.Data{Editor: scriptEditor(t, "Custom rules.")})

	require.NoError(t, handleAddCmd(cfg, "Custom"))
	assert.Equal(t, "Custom rules.", readFile(t, filepath.Join(storage, "instructions", "Custom.md")))

	assert.ErrorIs(t, handleAddCmd(cfg, "Custom"), os.ErrExist)
}
