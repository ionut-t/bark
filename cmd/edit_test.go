package cmd

import (
	"path/filepath"
	"testing"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/instructions"
	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleEditCmd(t *testing.T) {
	t.Run("needs a flag", func(t *testing.T) {
		isolateDirs(t)
		cfg := config.New("", config.Data{})
		err := handleEditCmd(withFlags(t, editCmd(staticConfig(cfg))), cfg, "")
		assert.EqualError(t, err, "must specify either commit, reviewer or instruction to edit")
	})

	t.Run("commit instructions", func(t *testing.T) {
		storage := isolateDirs(t)
		cfg := config.New("", config.Data{Editor: scriptEditor(t, "My commit rules.")})
		require.NoError(t, handleEditCmd(withFlags(t, editCmd(staticConfig(cfg)), "-c"), cfg, ""))
		assert.Equal(t, "My commit rules.", readFile(t, filepath.Join(storage, "commit.md")))
	})

	t.Run("reviewer by name", func(t *testing.T) {
		storage := isolateDirs(t)
		require.NoError(t, reviewers.Config(storage, false))
		cfg := config.New("", config.Data{Editor: scriptEditor(t, "New Yoda.")})
		require.NoError(t, handleEditCmd(withFlags(t, editCmd(staticConfig(cfg)), "-r"), cfg, "Yoda"))
		assert.Equal(t, "New Yoda.", readFile(t, filepath.Join(storage, "reviewers", "Yoda.md")))
	})

	t.Run("instruction by name", func(t *testing.T) {
		storage := isolateDirs(t)
		require.NoError(t, instructions.Config(storage, false))
		cfg := config.New("", config.Data{Editor: scriptEditor(t, "New Go.")})
		require.NoError(t, handleEditCmd(withFlags(t, editCmd(staticConfig(cfg)), "-i"), cfg, "Go"))
		assert.Equal(t, "New Go.", readFile(t, filepath.Join(storage, "instructions", "Go.md")))
	})

	t.Run("missing names are errors", func(t *testing.T) {
		isolateDirs(t)
		cfg := config.New("", config.Data{Editor: "true"})
		err := handleEditCmd(withFlags(t, editCmd(staticConfig(cfg)), "-r"), cfg, "Nobody")
		assert.ErrorContains(t, err, "error finding reviewer")
		err = handleEditCmd(withFlags(t, editCmd(staticConfig(cfg)), "-i"), cfg, "Nothing")
		assert.ErrorContains(t, err, "error finding instruction")
	})

	t.Run("failing editor", func(t *testing.T) {
		isolateDirs(t)
		cfg := config.New("", config.Data{Editor: "false"})
		err := handleEditCmd(withFlags(t, editCmd(staticConfig(cfg)), "-c"), cfg, "")
		assert.ErrorContains(t, err, "error opening editor")
	})
}
