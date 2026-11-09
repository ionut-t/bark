package enclosing

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

const before = `package app

func greet() string {
	return "hello"
}
`

const after = `package app

func greet() string {
	return "hello, world"
}
`

// newRepo builds a repo where "feature" changes greet() in app.go on top
// of "main". HEAD is feature.
func newRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	writeFile(t, dir, "app.go", before)
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "base")
	runGit(t, dir, "checkout", "-b", "feature")
	writeFile(t, dir, "app.go", after)
	runGit(t, dir, "commit", "-am", "change greet")
	t.Chdir(dir)
	return dir
}

// fakeGH puts a gh on PATH that answers `gh pr diff` with diff and
// `gh pr view` with a pull request whose head is headSHA.
func fakeGH(t *testing.T, diff, headSHA string) {
	t.Helper()
	bin := t.TempDir()
	writeFile(t, bin, "pr.diff", diff)
	script := `#!/bin/sh
case "$2" in
diff) cat "` + filepath.Join(bin, "pr.diff") + `" ;;
view) printf '{"number":1,"title":"t","body":"","headRefOid":"` + headSHA + `","commits":[]}' ;;
*) exit 1 ;;
esac
`
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// The context must come from the version of each file the diff's line
// numbers refer to.
func TestForReview(t *testing.T) {
	want := "func greet() string {\n\treturn \"hello, world\"\n}"

	t.Run("branch reads HEAD, not uncommitted changes", func(t *testing.T) {
		dir := newRepo(t)
		// Uncommitted lines above greet() move it in the working tree.
		writeFile(t, dir, "app.go", "package app\n\nvar a = 1\n\nvar b = 2\n\nfunc other() {}\n"+strings.TrimPrefix(after, "package app\n"))

		r, err := git.GetReviewDiff(t.Context(), git.BranchDiff("main"))
		require.NoError(t, err)
		got, err := ForReview(t.Context(), r)
		require.NoError(t, err)
		assert.Contains(t, got, want)
		assert.NotContains(t, got, "func other")
	})

	t.Run("pull request whose head isn't checked out", func(t *testing.T) {
		dir := newRepo(t)
		diff := runGit(t, dir, "diff", "main...feature") + "\n"
		head := runGit(t, dir, "rev-parse", "feature")
		runGit(t, dir, "checkout", "main")
		fakeGH(t, diff, head)

		r, err := git.GetReviewDiff(t.Context(), git.PRDiff("1"))
		require.NoError(t, err)
		got, err := ForReview(t.Context(), r)
		require.NoError(t, err)
		assert.Contains(t, got, want)
	})

	t.Run("related code: a change that calls greet gets its definition", func(t *testing.T) {
		dir := newRepo(t)
		writeFile(t, dir, "use.go", "package app\n\nfunc use() string {\n\treturn greet()\n}\n")
		runGit(t, dir, "add", ".")
		runGit(t, dir, "commit", "-m", "add use")

		r, err := git.GetReviewDiff(t.Context(), git.CommitDiff("HEAD"))
		require.NoError(t, err)
		got, err := ForReview(t.Context(), r)
		require.NoError(t, err)
		assert.Contains(t, got, "## Related code")
		assert.Contains(t, got, "### app.go:3-5 (definition of greet)\n\n3\tfunc greet() string {\n4\t\treturn \"hello, world\"\n5\t}\n")
	})

	t.Run("diff without a source", func(t *testing.T) {
		newRepo(t)
		got, err := ForReview(t.Context(), git.ReviewDiff{Diff: "diff --git a/app.go b/app.go\n--- a/app.go\n+++ b/app.go\n@@ -4 +4 @@\n-x\n+y\n"})
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

// Enclosing declarations are capped so a large diff doesn't send whole
// files of them (2,210 lines on PR #105's diff).
func TestDeclarationsForDiffBudget(t *testing.T) {
	var src, diff strings.Builder
	src.WriteString("package app\n")
	diff.WriteString("diff --git a/app.go b/app.go\n--- a/app.go\n+++ b/app.go\n")
	line := 2
	for i := range 20 {
		fmt.Fprintf(&src, "\nfunc f%d() {\n", i)
		line += 2
		fmt.Fprintf(&diff, "@@ -%d,0 +%d @@\n+\t_ = 0\n", line, line)
		for range 98 {
			src.WriteString("\t_ = 0\n")
			line++
		}
		src.WriteString("}\n")
		line++
	}
	read := func(context.Context, string) ([]byte, error) { return []byte(src.String()), nil }

	got, err := DeclarationsForDiff(t.Context(), diff.String(), read)
	require.NoError(t, err)

	assert.Equal(t, maxEnclosingLines/100, strings.Count(got, "func f"))
	assert.Contains(t, got, "func f0() {")
	assert.Contains(t, got, "func f7() {")
	assert.False(t, strings.Contains(got, "func f8() {"), "100-line declarations: 8 fit in 800 lines")
}
