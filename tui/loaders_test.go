package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ionut-t/bark/v2/internal/instructions"
	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoaders(t *testing.T) {
	storage := t.TempDir()
	require.NoError(t, reviewers.Config(storage, false))
	require.NoError(t, instructions.Config(storage, false))

	t.Run("reviewers", func(t *testing.T) {
		t.Chdir(t.TempDir())
		msg := loadReviewersCmd(storage)().(reviewersLoadedMsg)
		require.NoError(t, msg.reviewersErr)
		assert.NotEmpty(t, msg.reviewers)
		assert.Error(t, msg.fileErr, "no .bark/reviewer.md")
		assert.Nil(t, msg.fileReviewer)

		require.NoError(t, os.MkdirAll(".bark", 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(".bark", "reviewer.md"), []byte("Repo persona."), 0o644))
		msg = loadReviewersCmd(storage)().(reviewersLoadedMsg)
		require.NoError(t, msg.fileErr)
		assert.Equal(t, "Repo persona.", msg.fileReviewer.Prompt)
	})

	// From a subdirectory of a repository, the reviewer file at its root
	// is found.
	t.Run("reviewer file from a subdirectory", func(t *testing.T) {
		root := t.TempDir()
		t.Chdir(root)
		require.NoError(t, exec.Command("git", "init", "-q").Run())
		require.NoError(t, os.MkdirAll(".bark", 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(".bark", "reviewer.md"), []byte("Repo persona."), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Join("internal", "pkg"), 0o755))
		t.Chdir(filepath.Join(root, "internal", "pkg"))

		msg := loadReviewersCmd(storage)().(reviewersLoadedMsg)
		require.NoError(t, msg.fileErr)
		assert.Equal(t, "Repo persona.", msg.fileReviewer.Prompt)
	})

	t.Run("instructions", func(t *testing.T) {
		t.Chdir(t.TempDir())
		msg := loadReviewInstructionsCmd(storage)().(reviewInstructionsLoadedMsg)
		require.NoError(t, msg.instructionsErr)
		assert.NotEmpty(t, msg.instructions)
		require.NoError(t, msg.overrideErr)
		assert.Empty(t, msg.override)

		require.NoError(t, os.MkdirAll(".bark", 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(".bark", "review.md"), []byte("Repo rules."), 0o644))
		msg = loadReviewInstructionsCmd(storage)().(reviewInstructionsLoadedMsg)
		assert.Equal(t, "Repo rules.", msg.override)
	})

	t.Run("missing storage", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing")
		assert.Error(t, loadReviewersCmd(missing)().(reviewersLoadedMsg).reviewersErr)
		assert.Error(t, loadReviewInstructionsCmd(missing)().(reviewInstructionsLoadedMsg).instructionsErr)
	})

	t.Run("commits outside a repository", func(t *testing.T) {
		t.Chdir(t.TempDir())
		msg := loadCommitsCmd(5)().(commitsLoadedMsg)
		assert.Error(t, msg.err)
		assert.Empty(t, msg.commits)
	})
}
