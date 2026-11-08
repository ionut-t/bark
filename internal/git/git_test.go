package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestGetBaseBranch(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, dir string)
		want    string
		wantErr bool
	}{
		{
			name: "origin/HEAD set wins over local branches",
			setup: func(t *testing.T, dir string) {
				runGit(t, dir, "update-ref", "refs/remotes/origin/develop", "main")
				runGit(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/develop")
			},
			want: "develop",
		},
		{
			name: "origin/HEAD pointing at a branch with a slash keeps the full name",
			setup: func(t *testing.T, dir string) {
				runGit(t, dir, "update-ref", "refs/remotes/origin/release/2.x", "main")
				runGit(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/release/2.x")
			},
			want: "release/2.x",
		},
		{
			name: "origin/HEAD read even when a local branch has the same name as its target",
			setup: func(t *testing.T, dir string) {
				runGit(t, dir, "update-ref", "refs/remotes/origin/release/2.x", "main")
				runGit(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/release/2.x")
				// Makes "origin/release/2.x" ambiguous, so symbolic-ref --short
				// prints "remotes/origin/release/2.x".
				runGit(t, dir, "branch", "origin/release/2.x", "main")
			},
			want: "release/2.x",
		},
		{
			name:  "no origin/HEAD falls back to main",
			setup: func(t *testing.T, dir string) {},
			want:  "main",
		},
		{
			name: "no origin/HEAD falls back to master",
			setup: func(t *testing.T, dir string) {
				runGit(t, dir, "branch", "-m", "main", "master")
			},
			want: "master",
		},
		{
			name: "no origin/HEAD falls back to develop",
			setup: func(t *testing.T, dir string) {
				runGit(t, dir, "branch", "-m", "main", "develop")
			},
			want: "develop",
		},
		{
			name: "main preferred over master and develop",
			setup: func(t *testing.T, dir string) {
				runGit(t, dir, "branch", "master", "main")
				runGit(t, dir, "branch", "develop", "main")
			},
			want: "main",
		},
		{
			name: "no origin/HEAD and only a remote-tracking main",
			setup: func(t *testing.T, dir string) {
				runGit(t, dir, "update-ref", "refs/remotes/origin/main", "main")
				runGit(t, dir, "branch", "-D", "main")
			},
			want: "origin/main",
		},
		{
			name: "remote-tracking main preferred over local develop",
			setup: func(t *testing.T, dir string) {
				runGit(t, dir, "update-ref", "refs/remotes/origin/main", "main")
				runGit(t, dir, "branch", "-m", "main", "develop")
			},
			want: "origin/main",
		},
		{
			name: "local main preferred over remote-tracking main",
			setup: func(t *testing.T, dir string) {
				runGit(t, dir, "update-ref", "refs/remotes/origin/main", "main")
			},
			want: "main",
		},
		{
			name: "no candidate branch",
			setup: func(t *testing.T, dir string) {
				runGit(t, dir, "branch", "-m", "main", "trunk")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newDivergedRepo(t)
			tt.setup(t, dir)

			got, err := GetBaseBranch(t.Context())
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetBranchInfo_WithoutOriginHead(t *testing.T) {
	newDivergedRepo(t)

	info, err := GetBranchInfo(t.Context(), "", 1000)
	require.NoError(t, err)

	assert.Equal(t, "main", info.BaseBranch)
	require.Len(t, info.Commits, 1)
	assert.Equal(t, "add feature.txt", info.Commits[0].Message)
}

// newRepoWithPipeCommit builds a diverged repo (see newDivergedRepo) and adds a
// commit on feature whose author, subject and body all contain "|".
func newRepoWithPipeCommit(t *testing.T) string {
	t.Helper()
	dir := newDivergedRepo(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "pipe.txt"), []byte("pipe\n"), 0o644))
	runGit(t, dir, "add", "pipe.txt")
	runGit(t, dir, "commit",
		"--author", "Ann | Bob <ann@example.com>",
		"-m", "fix: a | b",
		"-m", "body | with pipe")
	return dir
}

func TestCommitParsing_PipeInFields(t *testing.T) {
	const (
		wantAuthor  = "Ann | Bob"
		wantMessage = "fix: a | b"
		wantBody    = "body | with pipe"
	)

	t.Run("GetCommits", func(t *testing.T) {
		newRepoWithPipeCommit(t)

		commits, err := GetCommits(t.Context(), 1)
		require.NoError(t, err)
		require.Len(t, commits, 1)
		assert.Len(t, commits[0].Hash, 40)
		assert.Equal(t, wantAuthor, commits[0].Author)
		assert.Equal(t, wantMessage, commits[0].Message)
	})

	t.Run("GetCommit", func(t *testing.T) {
		newRepoWithPipeCommit(t)

		commit, err := GetCommit(t.Context(), "HEAD")
		require.NoError(t, err)
		require.NotNil(t, commit)
		assert.Len(t, commit.Hash, 40)
		assert.Equal(t, wantAuthor, commit.Author)
		assert.Equal(t, wantMessage, commit.Message)
	})

	t.Run("GetBranchCommits", func(t *testing.T) {
		newRepoWithPipeCommit(t)

		commits, err := GetBranchCommits(t.Context(), "main")
		require.NoError(t, err)
		require.Len(t, commits, 2)

		assert.Len(t, commits[0].Hash, 40)
		assert.Equal(t, wantAuthor, commits[0].Author)
		assert.Equal(t, wantMessage, commits[0].Message)
		assert.Equal(t, wantBody, commits[0].Body)

		assert.Equal(t, "Test", commits[1].Author)
		assert.Equal(t, "add feature.txt", commits[1].Message)
		assert.Empty(t, commits[1].Body)
	})
}

func TestFormatGoVersionHeader(t *testing.T) {
	t.Parallel()

	goMod := []byte("module example.com/m\n\ngo 1.27.3 // pinned\n\ntoolchain go1.28.0\n")
	goDiff := "diff --git a/cmd/main.go b/cmd/main.go\n--- a/cmd/main.go\n+++ b/cmd/main.go\n"
	mdDiff := "diff --git a/README.md b/README.md\n--- a/README.md\n+++ b/README.md\n"

	tests := []struct {
		name  string
		goMod []byte
		diff  string
		want  string
	}{
		{"go diff", goMod, goDiff, "go.mod targets Go 1.27.3. The language features and standard library APIs of Go 1.27.3 and earlier exist"},
		{"no go files in diff", goMod, mdDiff, ""},
		{"no go directive", []byte("module example.com/m\n"), goDiff, ""},
		{"no go.mod", nil, goDiff, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := FormatGoVersionHeader(tt.goMod, tt.diff)
			if tt.want == "" {
				assert.Empty(t, got)
				return
			}
			assert.True(t, strings.HasPrefix(got, "## Go version\n\n"), got)
			assert.Contains(t, got, tt.want)
		})
	}
}
