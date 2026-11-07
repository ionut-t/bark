package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runGit runs a git command in dir and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

// commitFile writes content to name in dir and commits it.
func commitFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	runGit(t, dir, "add", name)
	runGit(t, dir, "commit", "-m", "add "+name)
}

// newDivergedRepo builds a repo where "feature" branched off "main", then both
// gained a commit: feature.txt on feature, main-only.txt on main. HEAD is feature.
func newDivergedRepo(t *testing.T) string {
	t.Helper()

	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	commitFile(t, dir, "base.txt", "base\n")

	runGit(t, dir, "checkout", "-b", "feature")
	commitFile(t, dir, "feature.txt", "feature\n")

	runGit(t, dir, "checkout", "main")
	commitFile(t, dir, "main-only.txt", "added on main after branching\n")

	runGit(t, dir, "checkout", "feature")
	t.Chdir(dir)
	return dir
}

func TestGetBranchDiff_UsesMergeBase(t *testing.T) {
	tests := []struct {
		name     string
		maxLines uint32
	}{
		{name: "unlimited", maxLines: 0},
		{name: "line limited", maxLines: 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newDivergedRepo(t)

			diff, err := GetBranchDiff(t.Context(), "main", tt.maxLines)
			require.NoError(t, err)

			assert.Contains(t, diff, "feature.txt", "branch's own change should be in the diff")
			assert.NotContains(t, diff, "main-only.txt", "commits added to the base after branching should not appear")
		})
	}
}

func TestGetBranchDiffStat_UsesMergeBase(t *testing.T) {
	newDivergedRepo(t)

	stat := GetBranchDiffStat(t.Context(), "main")

	assert.Contains(t, stat, "feature.txt")
	assert.NotContains(t, stat, "main-only.txt")
}

func TestGetBranchDiff_MatchesGetBranchStats(t *testing.T) {
	newDivergedRepo(t)

	stat := GetBranchDiffStat(t.Context(), "main")
	files, _, _, err := GetBranchStats(t.Context(), "main")
	require.NoError(t, err)

	assert.Equal(t, 1, files)
	assert.Contains(t, stat, "1 file changed", "diff stat and branch stats should agree")
}

// newRepoWithUntrackedFile builds a repo with one commit, a modified tracked file,
// a new untracked file, an ignored file and an untracked excluded (*.sum) file.
func newRepoWithUntrackedFile(t *testing.T) string {
	t.Helper()
	dir := newDivergedRepo(t)

	commitFile(t, dir, ".gitignore", "ignored.txt\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("feature changed\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("brand new\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("ignored\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.sum"), []byte("checksums\n"), 0o644))
	return dir
}

func TestGetWorkingTreeDiff_UntrackedFiles(t *testing.T) {
	tests := []struct {
		name          string
		all           bool
		wantUntracked bool
	}{
		{name: "all includes new files", all: true, wantUntracked: true},
		{name: "staged only skips new files", all: false, wantUntracked: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newRepoWithUntrackedFile(t)
			runGit(t, ".", "add", "feature.txt")

			diff, err := GetWorkingTreeDiff(t.Context(), tt.all)
			require.NoError(t, err)

			assert.Contains(t, diff, "feature.txt", "tracked change should be in the diff")
			if tt.wantUntracked {
				assert.Contains(t, diff, "diff --git a/new.txt b/new.txt")
				assert.Contains(t, diff, "+brand new")
			} else {
				assert.NotContains(t, diff, "new.txt")
			}
			assert.NotContains(t, diff, "ignored.txt", "gitignored files should not be reviewed")
			assert.NotContains(t, diff, "go.sum", "default excludes should apply to new files")
		})
	}
}

func TestGetWorkingTreeStat_UntrackedFiles(t *testing.T) {
	newRepoWithUntrackedFile(t)

	stat := GetWorkingTreeStat(t.Context(), true)

	assert.Contains(t, stat, "feature.txt")
	assert.Contains(t, stat, "new.txt")
	assert.NotContains(t, stat, "ignored.txt")
	assert.NotContains(t, stat, "go.sum")
	assert.Contains(t, stat, "2 files changed")
}

func TestGetWorkingTreeDiff_LeavesIndexUntouched(t *testing.T) {
	dir := newRepoWithUntrackedFile(t)

	_, err := GetWorkingTreeDiff(t.Context(), true)
	require.NoError(t, err)
	_ = GetWorkingTreeStat(t.Context(), true)

	cmd := exec.Command("git", "status", "--porcelain", "--", "new.txt")
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "?? new.txt\n", string(out), "new file should still be untracked in the user's index")
}

func TestGetWorkingTreeDiff_UntrackedFromSubdirectory(t *testing.T) {
	dir := newRepoWithUntrackedFile(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))
	t.Chdir(filepath.Join(dir, "sub"))

	diff, err := GetWorkingTreeDiff(t.Context(), true)
	require.NoError(t, err)

	assert.Contains(t, diff, "diff --git a/new.txt b/new.txt", "new files outside the current directory should be included")
}

func TestWithUntrackedIndex_NoIndexYet(t *testing.T) {
	newDivergedRepo(t)
	require.NoError(t, os.Remove(filepath.Join(".git", "index")))
	require.NoError(t, os.WriteFile("new.txt", []byte("brand new\n"), 0o644))

	cmd := exec.Command("git", "ls-files")
	cleanup, err := withUntrackedIndex(t.Context(), cmd)
	require.NoError(t, err)
	defer cleanup()

	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), "new.txt")
}
