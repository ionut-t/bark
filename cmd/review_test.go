package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPickPreferredReviewer(t *testing.T) {
	t.Parallel()

	assert.Empty(t, pickPreferredReviewer(nil, func(int) int { panic("not called") }))

	names := []string{"Yoda", "Linus", "Bob"}
	for i, want := range names {
		got := pickPreferredReviewer(names, func(n int) int {
			assert.Equal(t, len(names), n, "every name can be picked")
			return i
		})
		assert.Equal(t, want, got)
	}
}

// The review action and Bark's own workflow read the picked reviewer from
// Bark's messages for their "Reviewed by" label, so they must match the line
// Bark prints.
func TestReviewActionReadsPickedReviewer(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"../.github/actions/bark-review/action.yaml",
		"../.github/workflows/bark.yaml",
	} {
		script, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(script), "PICKED_PREFIX='"+pickedReviewerPrefix+"'", path)
	}
}

// The repository's reviewer, which beats the preferred reviewers, is found
// at the repository's root from a subdirectory too.
func TestRepoReviewerExists(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	require.NoError(t, exec.Command("git", "init", "-q").Run())
	sub := filepath.Join(root, "internal", "pkg")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	t.Chdir(sub)

	assert.False(t, repoReviewerExists(context.Background()), "no reviewer file")

	require.NoError(t, os.MkdirAll(filepath.Join(root, ".bark"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".bark", "reviewer.md"), []byte("Repo persona."), 0o644))
	assert.True(t, repoReviewerExists(context.Background()))
}
