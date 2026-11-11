package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseIgnore(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		ignored  []string
		reviewed []string
	}{
		{
			name: "defaults without a file",
			file: "",
			ignored: []string{
				"go.sum", "a/b/go.sum", "Cargo.lock", "api/x.pb.go", "api/x.pb.gw.go",
				"package-lock.json", "web/package-lock.json", "pnpm-lock.yaml", "bun.lockb", "npm-shrinkwrap.json",
				"static/app.min.js", "static/app.min.css",
			},
			reviewed: []string{"main.go", "sum.go", "lock/a.go", "package.json", "app.js", "min.js", "pnpm-workspace.yaml"},
		},
		{
			name:     "pattern without a slash matches at any depth",
			file:     "*.snap\n",
			ignored:  []string{"x.snap", "a/b/x.snap"},
			reviewed: []string{"x.snap.go", "snap/a.go"},
		},
		{
			name:     "trailing slash ignores a directory at any depth",
			file:     "vendor/\n",
			ignored:  []string{"vendor/a.go", "pkg/vendor/x/a.go"},
			reviewed: []string{"vendors/a.go", "vendor.go"},
		},
		{
			name:     "leading slash anchors at the repository root",
			file:     "/gen.go\n",
			ignored:  []string{"gen.go"},
			reviewed: []string{"a/gen.go"},
		},
		{
			name:     "a slash in the middle anchors too, and star stays in one directory",
			file:     "docs/*.md\n",
			ignored:  []string{"docs/a.md"},
			reviewed: []string{"docs/x/a.md", "a/docs/a.md"},
		},
		{
			name:     "double star crosses directories",
			file:     "docs/**/*.md\n",
			ignored:  []string{"docs/a.md", "docs/x/y/a.md"},
			reviewed: []string{"a/docs/a.md", "docs/a.txt"},
		},
		{
			name:     "question mark and character class",
			file:     "testdata/case?.[jt]son\n",
			ignored:  []string{"testdata/case1.json", "testdata/caseA.tson"},
			reviewed: []string{"testdata/case10.json", "testdata/case1.yson"},
		},
		{
			name:     "negating a default reviews those files again",
			file:     "!*.lock\n",
			ignored:  []string{"go.sum"},
			reviewed: []string{"Cargo.lock", "web/yarn.lock"},
		},
		{
			name:     "comments, blank lines and escaped hash",
			file:     "# generated\n\n   \n\\#notes.txt\n",
			ignored:  []string{"#notes.txt"},
			reviewed: []string{"generated", "notes.txt"},
		},
		{
			name:     "CRLF line endings and trailing spaces",
			file:     "*.snap  \r\n*.golden\r\n",
			ignored:  []string{"x.snap", "a/x.golden"},
			reviewed: []string{"x.snap  "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, err := parseIgnore(tt.file)
			require.NoError(t, err)

			for _, p := range tt.ignored {
				assert.True(t, rules.matches(p), "%s should be ignored", p)
			}
			for _, p := range tt.reviewed {
				assert.False(t, rules.matches(p), "%s should be reviewed", p)
			}
		})
	}
}

func TestParseIgnoreErrors(t *testing.T) {
	tests := []struct {
		name string
		file string
		want []string
	}{
		{
			name: "negating a pattern that isn't a default",
			file: "*.snap\n!*.txt\n",
			want: []string{"line 2", "!*.txt", "*.sum, *.lock, *.pb.go, *.pb.gw.go, package-lock.json"},
		},
		{
			name: "pattern naming the ignore file",
			file: ".barkignore\nevil.go\n",
			want: []string{"line 1", ".barkignore", "always reviewed"},
		},
		{
			name: "pattern matching the repository config",
			file: "*.snap\n.bark/\n",
			want: []string{"line 2", ".bark/config.toml", "always reviewed"},
		},
		{
			name: "broad patterns that would match them",
			file: ".*\n**\nconfig.toml\n*\n",
			want: []string{"line 1", "line 2", "line 3", "line 4"},
		},
		{
			name: "unclosed character class",
			file: "[abc\n",
			want: []string{"line 1", "[abc"},
		},
		{
			name: "every bad line is reported",
			file: "!x\nok\n[\n",
			want: []string{"line 1", "line 3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, err := parseIgnore(tt.file)
			require.Error(t, err)
			for _, w := range tt.want {
				assert.Contains(t, err.Error(), w)
			}
			// The valid lines still apply, so a bad line doesn't turn the
			// whole file off.
			assert.True(t, rules.matches("go.sum"))
			for _, p := range protectedPaths {
				assert.False(t, rules.matches(p), "%s must stay in the diff", p)
			}
		})
	}

	t.Run("other files in .bark can be ignored", func(t *testing.T) {
		rules, err := parseIgnore(".bark/prices.json\n")
		require.NoError(t, err)
		assert.True(t, rules.matches(".bark/prices.json"))
	})

	t.Run("valid lines still apply", func(t *testing.T) {
		rules, err := parseIgnore("!*.txt\n*.snap\n")
		require.Error(t, err)
		assert.True(t, rules.matches("a/x.snap"))
	})
}

// TestIgnorePathspecsMatchGit checks that the pathspecs given to git select
// the same files as the Go matcher used on pull request diffs.
func TestIgnorePathspecsMatchGit(t *testing.T) {
	file := strings.Join([]string{
		"*.snap", "vendor/", "/gen.go", "docs/*.md", "docs/**/*.md",
		"testdata/case?.[jt]son", "!*.lock", "\\#notes.txt",
	}, "\n")
	rules, err := parseIgnore(file)
	require.NoError(t, err)

	paths := []string{
		"main.go", "go.sum", "a/b/go.sum", "Cargo.lock", "web/yarn.lock", "api/x.pb.go",
		"x.snap", "a/b/x.snap", "x.snap.go", "vendor/a.go", "pkg/vendor/x/a.go", "vendors/a.go",
		"gen.go", "a/gen.go", "docs/a.md", "docs/x/a.md", "docs/x/y/a.md", "a/docs/a.md",
		"testdata/case1.json", "testdata/case10.json", "#notes.txt", "notes.txt",
	}

	dir := newDivergedRepo(t)
	for _, p := range paths {
		full := filepath.Join(dir, filepath.FromSlash(p))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte("x\n"), 0o644))
	}
	runGit(t, dir, "add", "-A")

	// From the root: with only exclude pathspecs, ls-files lists just the
	// current directory (diff uses the whole tree; see
	// TestBarkignoreAppliesToDiffs).
	cmd := exec.Command("git", append([]string{"ls-files", "--"}, rules.pathspecs()...)...)
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	var fromGit []string
	for line := range strings.Lines(string(out)) {
		fromGit = append(fromGit, strings.TrimSpace(line))
	}

	var fromGo []string
	for _, p := range append(paths, "base.txt", "feature.txt") {
		if !rules.matches(p) {
			fromGo = append(fromGo, p)
		}
	}

	slices.Sort(fromGit)
	slices.Sort(fromGo)
	assert.Equal(t, fromGo, fromGit)
}

// newRepoWithIgnoreFile builds a repo whose last commit adds files that
// .barkignore, the defaults and the negated default decide about.
func newRepoWithIgnoreFile(t *testing.T) string {
	t.Helper()
	dir := newDivergedRepo(t)
	commitFile(t, dir, ".barkignore", "*.snap\n!*.lock\n")
	for name, content := range map[string]string{
		"keep.go":    "package keep\n",
		"x.snap":     "snapshot\n",
		"Cargo.lock": "lock\n",
		"go.sum":     "sum\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "add files")
	return dir
}

func TestBarkignoreAppliesToDiffs(t *testing.T) {
	dir := newRepoWithIgnoreFile(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))
	t.Chdir(filepath.Join(dir, "sub"))

	diff, err := GetDiff(t.Context(), "HEAD")
	require.NoError(t, err)
	assert.Contains(t, diff, "keep.go")
	assert.Contains(t, diff, "Cargo.lock", "a negated default should be reviewed")
	assert.NotContains(t, diff, "x.snap", ".barkignore patterns should be left out")
	assert.NotContains(t, diff, "go.sum", "defaults should still apply")

	stat := GetCommitStat(t.Context(), "HEAD")
	assert.Contains(t, stat, "Cargo.lock")
	assert.NotContains(t, stat, "x.snap")

	branch, err := GetBranchDiff(t.Context(), "main", 0)
	require.NoError(t, err)
	assert.Contains(t, branch, "Cargo.lock")
	assert.NotContains(t, branch, "x.snap")
	assert.NotContains(t, GetBranchDiffStat(t.Context(), "main"), "x.snap")
}

func TestBarkignoreAppliesToWorkingTree(t *testing.T) {
	dir := newDivergedRepo(t)
	commitFile(t, dir, ".barkignore", "*.snap\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.snap"), []byte("snapshot\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "keep.go"), []byte("package keep\n"), 0o644))

	diff, err := GetWorkingTreeDiff(t.Context(), true)
	require.NoError(t, err)
	assert.Contains(t, diff, "keep.go")
	assert.NotContains(t, diff, "x.snap")

	stat := GetWorkingTreeStat(t.Context(), true)
	assert.Contains(t, stat, "keep.go")
	assert.NotContains(t, stat, "x.snap")
}

func TestFilterDiffMatchesWholePaths(t *testing.T) {
	rules, err := parseIgnore("vendor/\n!*.lock\n")
	require.NoError(t, err)

	diff := strings.Join([]string{
		"diff --git a/main.go b/main.go",
		"+main",
		"diff --git a/pkg/vendor/x.go b/pkg/vendor/x.go",
		"+vendored",
		"diff --git a/go.sum b/go.sum",
		"+sum",
		"diff --git a/Cargo.lock b/Cargo.lock",
		"+lock",
	}, "\n")

	got := filterDiff(diff, rules)
	assert.Contains(t, got, "+main")
	assert.Contains(t, got, "+lock")
	assert.NotContains(t, got, "+vendored")
	assert.NotContains(t, got, "+sum")
}

func TestCheckIgnoreFile(t *testing.T) {
	dir := newDivergedRepo(t)
	assert.NoError(t, CheckIgnoreFile(t.Context()), "no file is fine")

	require.NoError(t, os.WriteFile(filepath.Join(dir, IgnoreFileName), []byte("*.snap\n!*.txt\n"), 0o644))
	err := CheckIgnoreFile(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), IgnoreFileName)
	assert.Contains(t, err.Error(), "line 2")
}

// The files that configure Bark's review must stay in the diff, so a pull
// request can't hide its own files, or change how it's reviewed, unseen.
func TestProtectedPathsMatchTheConfigFiles(t *testing.T) {
	assert.Equal(t, []string{IgnoreFileName, filepath.ToSlash(filepath.Join(".bark", "config.toml"))}, protectedPaths)
	assert.Equal(t, filepath.Join("root", filepath.FromSlash(protectedPaths[1])), config.RepoFilePath("root"))
}

func TestBarkignoreCantHideItself(t *testing.T) {
	dir := newDivergedRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".bark"), 0o755))
	for name, content := range map[string]string{
		".barkignore":       ".barkignore\n.bark/\nevil.go\n",
		".bark/config.toml": "min_severity = 'critical'\n",
		"evil.go":           "package evil\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(content), 0o644))
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "hide things")

	diff, err := GetDiff(t.Context(), "HEAD")
	require.NoError(t, err)
	assert.Contains(t, diff, "b/.barkignore")
	assert.Contains(t, diff, "b/.bark/config.toml")
	assert.NotContains(t, diff, "b/evil.go", "valid lines still apply")
	assert.Error(t, CheckIgnoreFile(t.Context()))
}

// The totals in a pull request description's prompt must count the same
// files as the diff the model sees.
func TestBranchStatsLeaveOutIgnoredFiles(t *testing.T) {
	newRepoWithIgnoreFile(t)

	files, additions, _, err := GetBranchStats(t.Context(), "main")
	require.NoError(t, err)

	// feature.txt, .barkignore, keep.go and Cargo.lock (a negated default);
	// x.snap and go.sum are ignored.
	assert.Equal(t, 4, files)
	assert.Equal(t, 5, additions)
	assert.Contains(t, GetBranchDiffStat(t.Context(), "main"), "4 files changed", "stats and diff stat agree")
}
