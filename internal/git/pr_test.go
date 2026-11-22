package git

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ionut-t/bark/v2/internal/tokens"
)

// prView is what the fake gh answers to `gh pr view`.
type prView struct {
	Number       int      `json:"number"`
	Title        string   `json:"title"`
	Body         string   `json:"body"`
	HeadRefOid   string   `json:"headRefOid"`
	BaseRefOid   string   `json:"baseRefOid"`
	Commits      []any    `json:"commits"`
	Files        []prFile `json:"files"`
	ChangedFiles int      `json:"changedFiles"`
}

type prFile struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// fakeGH puts a gh on PATH. `gh pr view` prints view as JSON, or fails if
// view is nil; `gh pr diff` prints diff, or fails like GitHub does for a
// too-large diff if diff is "". `gh api` fails.
func fakeGH(t *testing.T, view *prView, diff string) {
	t.Helper()
	bin := t.TempDir()
	viewCmd := `echo "GraphQL: Could not resolve to a PullRequest" >&2; exit 1`
	if view != nil {
		out, err := json.Marshal(view)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(bin, "view.json"), out, 0o644))
		viewCmd = `cat "` + filepath.Join(bin, "view.json") + `"`
	}
	diffCmd := `echo "HTTP 406: Sorry, the diff exceeded the maximum number of lines (20000)" >&2; exit 1`
	if diff != "" {
		require.NoError(t, os.WriteFile(filepath.Join(bin, "pr.diff"), []byte(diff), 0o644))
		diffCmd = `cat "` + filepath.Join(bin, "pr.diff") + `"`
	}
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = pr ] && [ \"$2\" = view ]; then " + viewCmd + "; exit; fi\n" +
		"if [ \"$1\" = pr ] && [ \"$2\" = diff ]; then " + diffCmd + "; exit; fi\n" +
		"exit 1\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// revParse returns the commit rev names in the repository at dir.
func revParse(t *testing.T, dir, rev string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", rev)
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

// newPRRepo is newDivergedRepo checked out at main, as for reviewing the
// pull request of feature into main, with the pull request's view and diff.
func newPRRepo(t *testing.T) (dir string, view *prView, diff string) {
	t.Helper()
	dir = newDivergedRepo(t)
	runGit(t, dir, "checkout", "main")
	view = &prView{
		Number:       7,
		Title:        "Add feature",
		HeadRefOid:   revParse(t, dir, "feature"),
		BaseRefOid:   revParse(t, dir, "main"),
		Commits:      []any{},
		Files:        []prFile{{Path: "feature.txt", Additions: 1}},
		ChangedFiles: 1,
	}
	cmd := exec.Command("git", "diff", "main...feature")
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	return dir, view, string(out)
}

// A pull request's metadata can't be read: the review goes on, but says
// what it's missing.
func TestGetReviewDiff_PRMetaFailureIsReported(t *testing.T) {
	_, _, diff := newPRRepo(t)
	fakeGH(t, nil, diff)

	r, err := GetReviewDiff(t.Context(), PRDiff("7"))
	require.NoError(t, err)
	assert.Equal(t, diff, r.Diff)
	require.Len(t, r.Warnings, 1)
	assert.Contains(t, r.Warnings[0], "pull request #7")
	assert.Contains(t, r.Warnings[0], "Could not resolve to a PullRequest", "the warning says why")

	fakeGH(t, &prView{Number: 7, Title: "t", Commits: []any{}}, diff)
	r, err = GetReviewDiff(t.Context(), PRDiff("7"))
	require.NoError(t, err)
	assert.Empty(t, r.Warnings)
}

// A pull request review has a diffstat, like the other modes.
func TestGetReviewDiff_PRStat(t *testing.T) {
	t.Run("from the local commits", func(t *testing.T) {
		dir, view, diff := newPRRepo(t)
		commitFile(t, dir, ".barkignore", "*.snap\n")
		view.BaseRefOid = revParse(t, dir, "main")
		fakeGH(t, view, diff)

		r, err := GetReviewDiff(t.Context(), PRDiff("7"))
		require.NoError(t, err)
		assert.Contains(t, r.Stat, "feature.txt | 1 +")
		assert.Contains(t, r.Stat, "1 file changed, 1 insertion(+)")
		assert.NotContains(t, r.Stat, "main-only.txt", "the stat is from the merge base, like the diff")
	})

	t.Run("from GitHub when the commits aren't local", func(t *testing.T) {
		_, view, diff := newPRRepo(t)
		view.HeadRefOid = strings.Repeat("a", 40)
		view.BaseRefOid = strings.Repeat("b", 40)
		view.Files = []prFile{
			{Path: "a.go", Additions: 3, Deletions: 1},
			{Path: "x.snap", Additions: 9},
			{Path: "go.sum", Additions: 2},
			{Path: "b.go", Deletions: 2},
		}
		view.ChangedFiles = 4
		fakeGH(t, view, diff)
		require.NoError(t, os.WriteFile(".barkignore", []byte("*.snap\n"), 0o644))

		r, err := GetReviewDiff(t.Context(), PRDiff("7"))
		require.NoError(t, err)
		assert.Contains(t, r.Stat, "a.go | 4 +++-")
		assert.Contains(t, r.Stat, "b.go | 2 --")
		assert.NotContains(t, r.Stat, "x.snap", ".barkignore applies")
		assert.NotContains(t, r.Stat, "go.sum", "the default ignores apply")
		assert.Contains(t, r.Stat, "2 files changed, 3 insertions(+), 3 deletions(-)")
	})

	t.Run("GitHub lists only some files", func(t *testing.T) {
		_, view, diff := newPRRepo(t)
		view.HeadRefOid = strings.Repeat("a", 40)
		view.BaseRefOid = strings.Repeat("b", 40)
		view.Files = []prFile{{Path: "a.go", Additions: 1}}
		view.ChangedFiles = 101
		fakeGH(t, view, diff)

		r, err := GetReviewDiff(t.Context(), PRDiff("7"))
		require.NoError(t, err)
		assert.Contains(t, r.Stat, "a.go | 1 +")
		assert.Contains(t, r.Stat, "GitHub listed only 1 of the 101 changed files")
	})
}

// GitHub refuses the diff of a very large pull request; Bark diffs the
// pull request's commits locally if it has them.
func TestGetReviewDiff_PRDiffLocalFallback(t *testing.T) {
	t.Run("commits are local", func(t *testing.T) {
		dir, view, want := newPRRepo(t)
		fakeGH(t, view, "")

		r, err := GetReviewDiff(t.Context(), PRDiff("7"))
		require.NoError(t, err)
		assert.Equal(t, want, r.Diff, "the diff is from the merge base, like GitHub's")
		assert.NotContains(t, r.Diff, "main-only.txt")
		require.Len(t, r.Warnings, 1)
		assert.Contains(t, r.Warnings[0], "maximum number of lines", "the warning says why")
		assert.Contains(t, r.Warnings[0], "local")
		assert.Equal(t, view.HeadRefOid, r.prHead)

		t.Run("with the excludes and the limit", func(t *testing.T) {
			runGit(t, dir, "checkout", "feature")
			commitFile(t, dir, "x.snap", "snapshot\n")
			commitFile(t, dir, "a.txt", "small\n")
			commitFile(t, dir, "b.txt", strings.Repeat("a line of the big file\n", 100))
			view.HeadRefOid = revParse(t, dir, "feature")
			runGit(t, dir, "checkout", "main")
			// Like the other modes, the excludes come from the working tree.
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".barkignore"), []byte("*.snap\n"), 0o644))
			fakeGH(t, view, "")

			full, err := GetReviewDiff(t.Context(), PRDiff("7"))
			require.NoError(t, err)
			assert.Contains(t, full.Diff, "b.txt")
			assert.NotContains(t, full.Diff, "x.snap")
			assert.Nil(t, full.Cut)

			at := strings.Index(full.Diff, "diff --git a/b.txt") + 200
			r, err := GetReviewDiff(t.Context(), PRDiff("7").WithLimit(DiffLimit{Tokens: uint32(float64(at) / tokens.CharsPerToken)}))
			require.NoError(t, err)
			require.NotNil(t, r.Cut)
			assert.Equal(t, &DiffCut{Files: 3, Kept: 2, Partial: true, PartialFile: "b.txt", Dropped: []string{"feature.txt"}, Setting: "max_diff_tokens"}, r.Cut, "a.txt, b.txt and feature.txt")
		})
	})

	t.Run("commits aren't local", func(t *testing.T) {
		_, view, _ := newPRRepo(t)
		view.HeadRefOid = strings.Repeat("a", 40)
		fakeGH(t, view, "")

		_, err := GetReviewDiff(t.Context(), PRDiff("7"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maximum number of lines")
	})

	t.Run("no metadata", func(t *testing.T) {
		newPRRepo(t)
		fakeGH(t, nil, "")

		_, err := GetReviewDiff(t.Context(), PRDiff("7"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maximum number of lines")
	})
}

// The Go version comes from the pull request's go.mod, not the checked-out
// one.
func TestGetReviewDiff_PRGoVersion(t *testing.T) {
	dir := newDivergedRepo(t)
	runGit(t, dir, "checkout", "main")
	commitFile(t, dir, "go.mod", "module m\n\ngo 1.22\n")
	runGit(t, dir, "checkout", "feature")
	runGit(t, dir, "merge", "-q", "main")
	commitFile(t, dir, "go.mod", "module m\n\ngo 1.26\n")
	commitFile(t, dir, "a.go", "package m\n")
	runGit(t, dir, "checkout", "main")

	view := &prView{Number: 7, HeadRefOid: revParse(t, dir, "feature"), BaseRefOid: revParse(t, dir, "main"), Commits: []any{}}
	cmd := exec.Command("git", "diff", "main...feature")
	cmd.Dir = dir
	diff, err := cmd.Output()
	require.NoError(t, err)

	t.Run("at the pull request's head", func(t *testing.T) {
		fakeGH(t, view, string(diff))
		r, err := GetReviewDiff(t.Context(), PRDiff("7"))
		require.NoError(t, err)
		assert.Contains(t, r.ContextHeader, "go.mod targets Go 1.26")
	})

	t.Run("checked out when the head can't be read", func(t *testing.T) {
		fakeGH(t, &prView{Number: 7, HeadRefOid: strings.Repeat("a", 40), Commits: []any{}}, string(diff))
		r, err := GetReviewDiff(t.Context(), PRDiff("7"))
		require.NoError(t, err)
		assert.Contains(t, r.ContextHeader, "go.mod targets Go 1.22")
	})
}

// The stat made from GitHub's file list draws the same bars as git diff
// --stat with a 50-column graph, the width git uses on a wide terminal.
func TestFormatPRFileStatMatchesGit(t *testing.T) {
	dir := newDivergedRepo(t)
	lines := func(prefix string, n int) string {
		var sb strings.Builder
		for i := range n {
			fmt.Fprintf(&sb, "%s %d\n", prefix, i)
		}
		return sb.String()
	}
	// Each file starts with old lines; the commit replaces del of them
	// and adds add more.
	files := []PRFile{
		{Path: "a.txt", Additions: 1, Deletions: 1},
		{Path: "b.txt", Additions: 10},
		{Path: "c.txt", Additions: 19, Deletions: 1},
		{Path: "d.txt", Additions: 30, Deletions: 30},
		{Path: "e.txt", Additions: 300, Deletions: 100},
		{Path: "f.txt", Additions: 1000},
		{Path: "g.txt", Deletions: 7},
	}
	for _, f := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f.Path), []byte(lines("old", 100)), 0o644))
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "old")
	for _, f := range files {
		content := strings.Join(strings.SplitAfter(lines("old", 100), "\n")[f.Deletions:], "") + lines("new", f.Additions)
		require.NoError(t, os.WriteFile(filepath.Join(dir, f.Path), []byte(content), 0o644))
	}
	cmd := exec.Command("git", "diff", "--stat=200", "--stat-graph-width=50")
	cmd.Dir = dir
	want, err := cmd.Output()
	require.NoError(t, err)

	bars := func(stat string) map[string]string {
		got := map[string]string{}
		for line := range strings.SplitSeq(stat, "\n") {
			if name, rest, ok := strings.Cut(line, "|"); ok {
				fields := strings.Fields(rest)
				got[strings.TrimSpace(name)] = strings.Join(fields[1:], "")
			}
		}
		return got
	}
	require.Len(t, bars(string(want)), len(files))
	assert.Equal(t, bars(string(want)), bars(formatPRFileStat(&PRMeta{Files: files}, ignoreRules{})))
}

// newReviewedPR is newPRRepo after a review of the pull request at its
// head (reviewed), followed by one more commit on feature (second.txt).
func newReviewedPR(t *testing.T) (dir string, view *prView, diff, reviewed string) {
	t.Helper()
	dir, view, _ = newPRRepo(t)
	reviewed = view.HeadRefOid
	runGit(t, dir, "checkout", "feature")
	commitFile(t, dir, "second.txt", "second\n")
	runGit(t, dir, "checkout", "main")
	view.HeadRefOid = revParse(t, dir, "feature")
	cmd := exec.Command("git", "diff", "main...feature")
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	return dir, view, string(out), reviewed
}

// A pull request reviewed before is reviewed from the commit the last
// review was at.
func TestGetReviewDiff_PRSince(t *testing.T) {
	t.Run("only the new changes", func(t *testing.T) {
		dir, view, diff, reviewed := newReviewedPR(t)
		fakeGH(t, view, diff)

		r, err := GetReviewDiff(t.Context(), PRDiff("7").Since(reviewed, ""))
		require.NoError(t, err)
		assert.Equal(t, reviewed, r.Since)
		assert.Empty(t, r.FullReason)
		assert.Contains(t, r.Diff, "second.txt")
		assert.NotContains(t, r.Diff, "feature.txt", "reviewed before")
		assert.Contains(t, r.Stat, "second.txt")
		assert.NotContains(t, r.Stat, "feature.txt")
		require.Len(t, r.Commits, 1, "only the new commits")
		assert.Equal(t, "add second.txt", r.Commits[0].Message)
		assert.Equal(t, view.HeadRefOid, r.PRHead())
		assert.Contains(t, r.ContextHeader, "Add feature", "the pull request header stays")
		assert.Equal(t, revParse(t, dir, "feature~2"), r.PRMergeBase(), "where feature branched off main")

		// Comments can go on any line of the whole pull request's diff,
		// which GitHub accepts; not on lines only the new changes show.
		assert.Contains(t, r.CommentDiff(), "feature.txt")
		assert.Contains(t, r.CommentDiff(), "second.txt")
	})

	t.Run("same base as recorded", func(t *testing.T) {
		dir, view, diff, reviewed := newReviewedPR(t)
		fakeGH(t, view, diff)
		r, err := GetReviewDiff(t.Context(), PRDiff("7").Since(reviewed, revParse(t, dir, "feature~2")))
		require.NoError(t, err)
		assert.Equal(t, reviewed, r.Since)
	})

	t.Run("with the excludes and the limit", func(t *testing.T) {
		dir, view, diff, reviewed := newReviewedPR(t)
		runGit(t, dir, "checkout", "feature")
		commitFile(t, dir, "x.snap", "snapshot\n")
		commitFile(t, dir, "b.txt", strings.Repeat("a line of the big file\n", 100))
		view.HeadRefOid = revParse(t, dir, "feature")
		runGit(t, dir, "checkout", "main")
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".barkignore"), []byte("*.snap\n"), 0o644))
		fakeGH(t, view, diff)

		r, err := GetReviewDiff(t.Context(), PRDiff("7").Since(reviewed, "").WithLimit(DiffLimit{Tokens: 100}))
		require.NoError(t, err)
		assert.NotContains(t, r.Diff, "x.snap")
		require.NotNil(t, r.Cut)
		assert.Equal(t, 2, r.Cut.Files, "b.txt and second.txt")
	})

	t.Run("nothing changed", func(t *testing.T) {
		_, view, diff, _ := newReviewedPR(t)
		fakeGH(t, view, diff)

		r, err := GetReviewDiff(t.Context(), PRDiff("7").Since(view.HeadRefOid, ""))
		require.NoError(t, err)
		assert.Equal(t, view.HeadRefOid, r.Since)
		assert.Empty(t, r.Diff)
		assert.Empty(t, r.Commits)
	})

	full := func(t *testing.T, view *prView, diff, since, base, reason string) {
		t.Helper()
		fakeGH(t, view, diff)
		r, err := GetReviewDiff(t.Context(), PRDiff("7").Since(since, base))
		require.NoError(t, err)
		assert.Empty(t, r.Since)
		assert.Equal(t, diff, r.Diff, "the whole pull request")
		assert.Contains(t, r.FullReason, reason)
		assert.Equal(t, r.Diff, r.CommentDiff(), "comments go on the diff reviewed")
	}

	t.Run("rebased", func(t *testing.T) {
		dir, view, _, reviewed := newReviewedPR(t)
		runGit(t, dir, "checkout", "feature")
		runGit(t, dir, "rebase", "main")
		view.HeadRefOid = revParse(t, dir, "feature")
		view.BaseRefOid = revParse(t, dir, "main")
		runGit(t, dir, "checkout", "main")
		full(t, view, "diff --git a/x b/x\n", reviewed, "", "no longer in the pull request's history (rebase or force-push)")
	})

	t.Run("base changed", func(t *testing.T) {
		// A stacked pull request retargeted at another branch: the commits
		// are the same, but what it changes isn't.
		dir, view, diff, reviewed := newReviewedPR(t)
		recordedBase := revParse(t, dir, "feature~2")
		view.BaseRefOid = revParse(t, dir, "feature~1")
		full(t, view, diff, reviewed, recordedBase, "the pull request's base changed since")
	})

	t.Run("reviewed commit isn't local", func(t *testing.T) {
		_, view, diff, _ := newReviewedPR(t)
		full(t, view, diff, strings.Repeat("c", 40), "", "isn't in the local repository")
	})

	t.Run("head isn't local", func(t *testing.T) {
		_, view, diff, reviewed := newReviewedPR(t)
		view.HeadRefOid = strings.Repeat("a", 40)
		full(t, view, diff, reviewed, "", "head commit")
	})

	t.Run("base merged in", func(t *testing.T) {
		dir, view, _, reviewed := newReviewedPR(t)
		runGit(t, dir, "checkout", "feature")
		runGit(t, dir, "merge", "--no-edit", "main")
		view.HeadRefOid = revParse(t, dir, "feature")
		runGit(t, dir, "checkout", "main")
		full(t, view, "diff --git a/x b/x\n", reviewed, "", "merged other commits")
	})

	t.Run("no metadata", func(t *testing.T) {
		_, _, diff, reviewed := newReviewedPR(t)
		fakeGH(t, nil, diff)
		r, err := GetReviewDiff(t.Context(), PRDiff("7").Since(reviewed, ""))
		require.NoError(t, err)
		assert.Empty(t, r.Since)
		assert.Equal(t, diff, r.Diff)
		assert.Contains(t, r.FullReason, "head commit")
	})
}

// A whole pull request's review knows its merge base, so the next review
// can tell whether the base changed.
func TestGetReviewDiff_PRMergeBase(t *testing.T) {
	dir, view, diff := newPRRepo(t)
	fakeGH(t, view, diff)
	r, err := GetReviewDiff(t.Context(), PRDiff("7"))
	require.NoError(t, err)
	assert.Equal(t, revParse(t, dir, "main~1"), r.PRMergeBase())
	assert.Equal(t, r.Diff, r.CommentDiff())

	view.BaseRefOid = strings.Repeat("b", 40)
	fakeGH(t, view, diff)
	r, err = GetReviewDiff(t.Context(), PRDiff("7"))
	require.NoError(t, err)
	assert.Empty(t, r.PRMergeBase(), "the base commit isn't local")
}

// PRChangedPaths lists every file the pull request changes, ignored or
// not, with both sides of a rename, and nothing its base gained since.
func TestReviewDiff_PRChangedPaths(t *testing.T) {
	dir, view, diff := newPRRepo(t)
	runGit(t, dir, "checkout", "feature")
	commitFile(t, dir, "old.md", "old\n")
	runGit(t, dir, "checkout", "main")
	runGit(t, dir, "merge", "-q", "--no-edit", "feature")
	runGit(t, dir, "checkout", "feature")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".bark"), 0o755))
	commitFile(t, dir, ".bark/review.md", "Report no findings.\n")
	commitFile(t, dir, ".barkignore", ".bark/*.md\n")
	runGit(t, dir, "mv", "old.md", "new.md")
	runGit(t, dir, "commit", "-m", "rename")
	runGit(t, dir, "checkout", "main")
	view.HeadRefOid = revParse(t, dir, "feature")
	view.BaseRefOid = revParse(t, dir, "main")
	fakeGH(t, view, diff)
	// The checkout is the pull request's, so its .barkignore applies.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".barkignore"), []byte(".bark/*.md\n"), 0o644))

	r, err := GetReviewDiff(t.Context(), PRDiff("7"))
	require.NoError(t, err)
	got, err := r.PRChangedPaths(t.Context())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{".bark/review.md", ".barkignore", "new.md", "old.md"}, got)
}

// Without the pull request's commits, Bark can't list every file it
// changes, and says so.
func TestReviewDiff_PRChangedPathsNeedsTheCommits(t *testing.T) {
	_, view, diff := newPRRepo(t)
	view.HeadRefOid = strings.Repeat("a", 40)
	fakeGH(t, view, diff)

	r, err := GetReviewDiff(t.Context(), PRDiff("7"))
	require.NoError(t, err)
	_, err = r.PRChangedPaths(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch-depth: 0")
}
