package cmd

import (
	"os"
	"testing"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/instructions"
	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleDeleteCmd(t *testing.T) {
	cfg := config.New("", config.Data{})

	t.Run("needs a flag", func(t *testing.T) {
		isolateDirs(t)
		err := handleDeleteCmd(withFlags(t, deleteCmd(staticConfig(cfg))), cfg, "x")
		assert.EqualError(t, err, "must specify either reviewer or instruction to delete")
	})

	t.Run("reviewer by name", func(t *testing.T) {
		storage := isolateDirs(t)
		require.NoError(t, reviewers.Config(storage, false))
		require.NoError(t, handleDeleteCmd(withFlags(t, deleteCmd(staticConfig(cfg)), "-r"), cfg, "Yoda"))
		_, err := reviewers.GetPath(storage, "Yoda")
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("instruction by name", func(t *testing.T) {
		storage := isolateDirs(t)
		require.NoError(t, instructions.Config(storage, false))
		require.NoError(t, handleDeleteCmd(withFlags(t, deleteCmd(staticConfig(cfg)), "-i"), cfg, "Go"))
		_, err := instructions.GetPath(storage, "Go")
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("missing names are errors", func(t *testing.T) {
		isolateDirs(t)
		err := handleDeleteCmd(withFlags(t, deleteCmd(staticConfig(cfg)), "-r"), cfg, "Nobody")
		assert.ErrorContains(t, err, "error deleting reviewer")
		err = handleDeleteCmd(withFlags(t, deleteCmd(staticConfig(cfg)), "-i"), cfg, "Nothing")
		assert.ErrorContains(t, err, "error deleting instruction")
	})
}
