package houserules

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repo is a fake repository: path -> contents. Reading a missing path fails.
type repo map[string]string

func (r repo) read(_ context.Context, path string) ([]byte, error) {
	if c, ok := r[path]; ok {
		return []byte(c), nil
	}
	return nil, os.ErrNotExist
}

func paths(rules Rules) []string {
	var out []string
	for _, s := range rules.Sources {
		out = append(out, s.Path)
	}
	return out
}

func TestLoad_DefaultsAtRoot(t *testing.T) {
	t.Parallel()

	r := repo{"AGENTS.md": "Use tabs.\n", "CLAUDE.md": "No panics.\n", "CONTRIBUTING.md": "Sign the CLA.\n"}
	got := Load(context.Background(), r.read, Options{Changed: []string{"main.go"}})

	assert.Equal(t, []string{"AGENTS.md", "CLAUDE.md"}, paths(got), "CONTRIBUTING.md isn't a default")
	assert.Contains(t, got.Text, "<file path=\"AGENTS.md\">\nUse tabs.\n</file>")
	assert.Contains(t, got.Text, "<file path=\"CLAUDE.md\">\nNo panics.\n</file>")
	assert.Less(t, strings.Index(got.Text, "AGENTS.md"), strings.Index(got.Text, "CLAUDE.md"))
	assert.Empty(t, got.Warnings())
}

func TestLoad_EmptyListUsesDefaults(t *testing.T) {
	t.Parallel()

	r := repo{"AGENTS.md": "a\n", "CLAUDE.md": "c\n"}
	got := Load(context.Background(), r.read, Options{Files: []string{}, Changed: []string{"x.go"}})

	assert.Equal(t, []string{"AGENTS.md", "CLAUDE.md"}, paths(got))
}

func TestLoad_NoFilesNoText(t *testing.T) {
	t.Parallel()

	got := Load(context.Background(), repo{}.read, Options{Changed: []string{"x.go"}})

	assert.Empty(t, got.Text)
	assert.Empty(t, got.Sources)
	assert.Empty(t, got.Warnings(), "missing default files aren't worth a warning")
	assert.Empty(t, got.Summary())
}

func TestLoad_NestedClosestFirstThenRoot(t *testing.T) {
	t.Parallel()

	r := repo{
		"AGENTS.md":         "root\n",
		"pkg/AGENTS.md":     "pkg\n",
		"pkg/api/AGENTS.md": "api\n",
		"other/AGENTS.md":   "not on a changed path\n",
		"pkg/api/CLAUDE.md": "api claude\n",
	}
	got := Load(context.Background(), r.read, Options{Changed: []string{"pkg/api/handler.go", "pkg/api/v1/route.go"}})

	// List order first; for each name, the folder closest to the changed
	// code first and the root last.
	assert.Equal(t, []string{"pkg/api/AGENTS.md", "pkg/AGENTS.md", "AGENTS.md", "pkg/api/CLAUDE.md"}, paths(got))
}

func TestLoad_PathsAreExact(t *testing.T) {
	t.Parallel()

	r := repo{
		"AGENTS.md":               "root\n",
		"pkg/AGENTS.md":           "nested\n",
		"docs/STYLE.md":           "style\n",
		".github/CONTRIBUTING.md": "contrib\n",
		"pkg/docs/STYLE.md":       "not this one\n",
	}
	got := Load(context.Background(), r.read, Options{
		Files:   []string{"./AGENTS.md", "docs/STYLE.md", ".github/CONTRIBUTING.md"},
		Changed: []string{"pkg/x.go", "pkg/docs/y.go"},
	})

	assert.Equal(t, []string{"AGENTS.md", "docs/STYLE.md", ".github/CONTRIBUTING.md"}, paths(got))
}

func TestLoad_ListedFileMissingWarns(t *testing.T) {
	t.Parallel()

	r := repo{"AGENTS.md": "a\n"}
	got := Load(context.Background(), r.read, Options{
		Files:   []string{"AGENTS.md", "docs/STYLE.md", "RULES.md"},
		Changed: []string{"pkg/x.go"},
	})

	assert.Equal(t, []string{"AGENTS.md"}, paths(got))
	assert.Equal(t, []string{"house rules: docs/STYLE.md, RULES.md not found"}, got.Warnings())
}

func TestLoad_SameContentOnce(t *testing.T) {
	t.Parallel()

	r := repo{"AGENTS.md": "Use tabs.\n", "CLAUDE.md": "Use tabs.\n\n"}
	got := Load(context.Background(), r.read, Options{Changed: []string{"x.go"}})

	assert.Equal(t, []string{"AGENTS.md"}, paths(got))
}

func TestLoad_FollowsAPointerFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		claude string
	}{
		// git show prints a symlink's target.
		{"symlink at a revision", "AGENTS.md"},
		// CLAUDE.md importing AGENTS.md and nothing else.
		{"lone import", "@AGENTS.md\n"},
		{"lone import of a relative path", "@./AGENTS.md\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := repo{"pkg/AGENTS.md": "Use tabs.\n", "pkg/CLAUDE.md": tt.claude}
			got := Load(context.Background(), r.read, Options{Files: []string{"pkg/CLAUDE.md"}, Changed: []string{"pkg/x.go"}})

			assert.Equal(t, []string{"pkg/CLAUDE.md"}, paths(got))
			assert.Contains(t, got.Text, "<file path=\"pkg/CLAUDE.md\">\nUse tabs.\n</file>")
		})
	}
}

func TestLoad_ImportsAmongOtherTextStay(t *testing.T) {
	t.Parallel()

	r := repo{"CLAUDE.md": "See @AGENTS.md.\nNo panics.\n", "AGENTS.md": "secret\n"}
	got := Load(context.Background(), r.read, Options{Files: []string{"CLAUDE.md"}, Changed: []string{"x.go"}})

	assert.Contains(t, got.Text, "See @AGENTS.md.\nNo panics.\n")
	assert.NotContains(t, got.Text, "secret")
}

func TestLoad_CutAtParagraphAndLeaveOutTheRest(t *testing.T) {
	t.Parallel()

	para := strings.Repeat("x", 90) + "\n"
	first := strings.Repeat(para, 2) + "\n" + strings.Repeat(para, 2) + "\n" + strings.Repeat(para, 2)
	r := repo{"AGENTS.md": "short\n", "CLAUDE.md": first, "docs/STYLE.md": "never shown\n"}

	// 200 tokens = 500 bytes: AGENTS.md and the first two paragraphs of
	// CLAUDE.md fit, the third doesn't.
	got := Load(context.Background(), r.read, Options{
		Files:     []string{"AGENTS.md", "CLAUDE.md", "docs/STYLE.md"},
		Changed:   []string{"x.go"},
		MaxTokens: 200,
	})

	require.Len(t, got.Sources, 3)
	assert.False(t, got.Sources[0].Cut())
	assert.True(t, got.Sources[1].Cut())
	assert.Positive(t, got.Sources[1].Tokens)
	assert.Zero(t, got.Sources[2].Tokens, "files after a cut are left out")
	assert.Contains(t, got.Text, strings.Repeat(para, 2)+"\n"+strings.Repeat(para, 2)+"[cut]\n</file>")
	assert.NotContains(t, got.Text, "never shown")
	assert.LessOrEqual(t, len(got.Text), 500+200, "the budget covers the files, give or take the tags")
	assert.Equal(t, []string{"house rules: CLAUDE.md cut, docs/STYLE.md left out to fit 200 tokens"}, got.Warnings())
	assert.Regexp(t, `^AGENTS\.md \(\d+\), CLAUDE\.md \(cut \d+/\d+\), docs/STYLE\.md \(left out, \d+\)$`, got.Summary())
}

func TestLoad_FileWithNoBoundaryInBudgetIsLeftOut(t *testing.T) {
	t.Parallel()

	r := repo{"AGENTS.md": strings.Repeat("y", 2000) + "\n"}
	got := Load(context.Background(), r.read, Options{Changed: []string{"x.go"}, MaxTokens: 100})

	require.Len(t, got.Sources, 1)
	assert.Zero(t, got.Sources[0].Tokens)
	assert.Empty(t, got.Text)
	assert.Equal(t, []string{"house rules: AGENTS.md left out to fit 100 tokens"}, got.Warnings())
}

func TestLoad_ReadErrorsCountAsMissing(t *testing.T) {
	t.Parallel()

	read := func(context.Context, string) ([]byte, error) { return nil, errors.New("git show: exit status 128") }
	got := Load(context.Background(), read, Options{Files: []string{"AGENTS.md"}, Changed: []string{"x.go"}})

	assert.Empty(t, got.Sources)
	assert.Equal(t, []string{"house rules: AGENTS.md not found"}, got.Warnings())
}

func TestValidateFile(t *testing.T) {
	t.Parallel()

	for _, ok := range []string{"AGENTS.md", "./AGENTS.md", "docs/STYLE.md", ".github/CONTRIBUTING.md"} {
		assert.NoError(t, ValidateFile(ok), ok)
	}
	for _, bad := range []string{"", "/etc/passwd", "../AGENTS.md", "docs/../../x.md", "docs/", "."} {
		assert.Error(t, ValidateFile(bad), bad)
	}
}

func TestChangedPaths(t *testing.T) {
	t.Parallel()

	diff := `diff --git a/pkg/a.go b/pkg/a.go
index 1..2 100644
--- a/pkg/a.go
+++ b/pkg/a.go
@@ -1,3 +1,3 @@
--- not a header: a removed SQL comment
+++ not a header either
diff --git a/old/gone.go b/old/gone.go
deleted file mode 100644
--- a/old/gone.go
+++ /dev/null
@@ -1 +0,0 @@
-x
diff --git a/from/x.go b/to/x.go
similarity index 90%
rename from from/x.go
rename to to/x.go
--- a/from/x.go
+++ b/to/x.go
diff --git "a/caf\303\251/x.go" "b/caf\303\251/x.go"
--- "a/caf\303\251/x.go"
+++ "b/caf\303\251/x.go"
`
	assert.Equal(t, []string{"pkg/a.go", "old/gone.go", "from/x.go", "to/x.go", "café/x.go"}, ChangedPaths(diff))
}

func TestLogField(t *testing.T) {
	t.Parallel()

	r := repo{"AGENTS.md": "Use tabs.\n"}
	assert.Equal(t, "off", LogField(false, Load(context.Background(), r.read, Options{})))
	assert.Equal(t, "none found", LogField(true, Load(context.Background(), repo{}.read, Options{})))
	assert.Equal(t, "AGENTS.md (4)", LogField(true, Load(context.Background(), r.read, Options{})))
}

// Whatever the budget, a file whose block ends in [cut] is reported as
// cut, a whole one as whole, and one that's left out has no block.
func TestLoad_CutFilesAreReportedCut(t *testing.T) {
	t.Parallel()

	block := regexp.MustCompile(`(?s)<file path="([^"]+)">\n(.*?)</file>`)
	r := repo{"AGENTS.md": "a\nb\n", "CLAUDE.md": "one\ntwo\n\nthree four five\nsix\n\nseven eight nine ten\n"}
	for maxTokens := 1; maxTokens <= 60; maxTokens++ {
		got := Load(context.Background(), r.read, Options{Changed: []string{"x.go"}, MaxTokens: maxTokens})

		blocks := map[string]string{}
		for _, m := range block.FindAllStringSubmatch(got.Text, -1) {
			blocks[m[1]] = m[2]
		}
		for _, src := range got.Sources {
			body, ok := blocks[src.Path]
			assert.Equal(t, ok, src.Tokens > 0, "budget %d: %s in the text", maxTokens, src.Path)
			if ok {
				assert.Equal(t, strings.HasSuffix(body, "[cut]\n"), src.Cut(), "budget %d: %s cut", maxTokens, src.Path)
				assert.LessOrEqual(t, src.Tokens, src.FullTokens, "budget %d: %s", maxTokens, src.Path)
			}
		}
	}
}

// Every file a pointer points to is recorded, so a change to it can be
// told apart from a change to an unrelated file: also when its text
// repeats a file read earlier, is empty, or can't be read.
func TestLoad_RecordsPointerTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		r    repo
		want []string
	}{
		{name: "read", r: repo{"pkg/CLAUDE.md": "@AGENTS.md\n", "pkg/AGENTS.md": "Use tabs.\n", "STYLE.md": "Root.\n"},
			want: []string{"pkg/AGENTS.md"}},
		{name: "same text as a file read earlier", r: repo{"pkg/CLAUDE.md": "@rules.md\n", "pkg/rules.md": "Root.\n", "STYLE.md": "Root.\n"},
			want: []string{"pkg/rules.md"}},
		{name: "empty", r: repo{"pkg/CLAUDE.md": "@rules.md\n", "pkg/rules.md": "", "STYLE.md": "Root.\n"},
			want: []string{"pkg/rules.md"}},
		{name: "missing", r: repo{"pkg/CLAUDE.md": "@rules.md\n", "STYLE.md": "Root.\n"},
			want: []string{"pkg/rules.md"}},
		{name: "no pointer", r: repo{"pkg/CLAUDE.md": "Use tabs.\n", "STYLE.md": "Root.\n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Load(context.Background(), tt.r.read, Options{Files: []string{"./STYLE.md", "pkg/CLAUDE.md"}, Changed: []string{"pkg/x.go"}})
			assert.Equal(t, tt.want, got.Targets)
		})
	}
}

func TestIsRulesFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		path  string
		files []string
		want  bool
	}{
		{name: "default at the root", path: "AGENTS.md", want: true},
		{name: "default in a folder", path: "pkg/sub/CLAUDE.md", want: true},
		{name: "not a default", path: "CONTRIBUTING.md"},
		{name: "a name only in a longer one", path: "pkg/MY-AGENTS.md"},
		{name: "listed name in a folder", path: "web/STYLE.md", files: []string{"STYLE.md"}, want: true},
		{name: "listed files replace the defaults", path: "AGENTS.md", files: []string{"STYLE.md"}},
		{name: "listed path", path: "docs/STYLE.md", files: []string{"docs/STYLE.md"}, want: true},
		{name: "listed path from the root only", path: "web/docs/STYLE.md", files: []string{"docs/STYLE.md"}},
		{name: "listed root path", path: "AGENTS.md", files: []string{"./AGENTS.md"}, want: true},
		{name: "listed root path isn't looked for in folders", path: "pkg/AGENTS.md", files: []string{"./AGENTS.md"}},
		{name: "invalid entries are skipped", path: "x.md", files: []string{"../x.md", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, IsRulesFile(tt.path, tt.files))
		})
	}
}
