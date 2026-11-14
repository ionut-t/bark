package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

// Regenerating a commit message must re-read the repository, so changes made
// while the commit view is open end up in the prompt.
func TestCommitRetryReloadsWorkingTree(t *testing.T) {
	for _, commitAll := range []bool{false, true} {
		t.Run(map[bool]string{false: "staged", true: "all"}[commitAll], func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)

			gitRun(t, dir, "init", "-q", "-b", "feat/reload")
			require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644))
			gitRun(t, dir, "add", ".")
			gitRun(t, dir, "commit", "-q", "-m", "init")

			m := &Model{
				config:      config.New("", config.Data{}),
				currentView: viewCommitChanges,
			}
			m.commitChanges = newCommitChangesModel(nil, "system", "stale diff", commitAll, 80, 24)

			// A change made after the commit view was opened.
			require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("two\n"), 0o644))
			gitRun(t, dir, "add", ".")

			_, cmd := m.handleCommitMessageRetry()
			require.NotNil(t, cmd)

			got := cmd()
			msg, ok := got.(commitDataLoadedMsg)
			require.True(t, ok, "retry should reload the commit data, got %T", got)
			require.NoError(t, msg.err)
			assert.Equal(t, commitAll, msg.commitAll)
			assert.Equal(t, "feat/reload", msg.branch)
			assert.Contains(t, msg.diff, "+two")
		})
	}
}
