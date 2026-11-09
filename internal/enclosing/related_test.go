package enclosing

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRepo is a repository in memory: path -> content.
type fakeRepo map[string]string

func (f fakeRepo) NewSideFile(_ context.Context, p string) ([]byte, error) {
	if c, ok := f[p]; ok {
		return []byte(c), nil
	}
	return nil, errors.New("no such file")
}

func (f fakeRepo) RepoFile(ctx context.Context, p string) ([]byte, error) {
	return f.NewSideFile(ctx, p)
}

func (f fakeRepo) Grep(_ context.Context, word string, globs ...string) ([]string, error) {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
	var out []string
	for p, c := range f {
		match := false
		for _, g := range globs {
			if ok, _ := path.Match(g, path.Base(p)); ok {
				match = true
			}
		}
		if match && re.MatchString(c) {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out, nil
}

// addDiff returns a diff that adds lines first..last of p's content, as if
// they were new.
func addDiff(repo fakeRepo, p string, first, last int) string {
	lines := strings.Split(repo[p], "\n")
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n@@ -%d,0 +%d,%d @@\n", p, p, p, p, first-1, first, last-first+1)
	for _, l := range lines[first-1 : last] {
		b.WriteString("+" + l + "\n")
	}
	return b.String()
}

// lineOf returns the 1-based line of the first line containing s.
func lineOf(t *testing.T, content, s string) int {
	t.Helper()
	for i, l := range strings.Split(content, "\n") {
		if strings.Contains(l, s) {
			return i + 1
		}
	}
	t.Fatalf("%q not found", s)
	return 0
}

func labels(pieces []Piece) []string {
	var out []string
	for _, p := range pieces {
		out = append(out, fmt.Sprintf("%s:%d-%d %s", p.Path, p.Start, p.End, p.Label))
	}
	return out
}

func findRelated(t *testing.T, repo fakeRepo, diff string, opt RelatedOptions) []Piece {
	t.Helper()
	if opt.MaxLines == 0 {
		opt.MaxLines = 1000
	}
	return Related(t.Context(), diff, repo, opt)
}

const appGo = `package app

import "example.com/m/store"

func Run(s *store.Store) string {
	v := s.Load("key")
	n := len(v)
	return helper(v, n)
}
`

const helperGo = `package app

func helper(v string, n int) string {
	return v
}
`

const storeGo = `package store

type Store struct {
	data map[string]string
}

// Load is nil-safe.
func (s *Store) Load(key string) string {
	if s == nil {
		return ""
	}
	return s.data[key]
}
`

func TestRelated_Definitions(t *testing.T) {
	repo := fakeRepo{"app/app.go": appGo, "app/helper.go": helperGo, "store/store.go": storeGo}
	run := lineOf(t, appGo, "func Run")

	got := findRelated(t, repo, addDiff(repo, "app/app.go", run, run+4), RelatedOptions{})

	// len is a builtin; *store.Store in the signature isn't a call.
	assert.Equal(t, []string{
		"store/store.go:8-13 definition of Load",
		"app/helper.go:3-5 definition of helper",
	}, labels(got))
	assert.Equal(t, strings.Split(strings.TrimSpace(helperGo), "\n")[2:5], got[1].Lines)
}

func TestRelated_OnlyAddedLinesAreSearchedForCalls(t *testing.T) {
	repo := fakeRepo{"app/app.go": appGo, "app/helper.go": helperGo, "store/store.go": storeGo}
	line := lineOf(t, appGo, "helper(v, n)")

	got := findRelated(t, repo, addDiff(repo, "app/app.go", line, line), RelatedOptions{})

	assert.Equal(t, []string{"app/helper.go:3-5 definition of helper"}, labels(got))
}

func TestRelated_SameNamedDefinitions(t *testing.T) {
	load := func(pkg string) string {
		return "package " + pkg + "\n\nfunc Load() string {\n\treturn \"" + pkg + "\"\n}\n"
	}
	caller := func(call string) string {
		return "package app\n\nfunc Run() {\n\t" + call + "\n}\n"
	}

	tests := []struct {
		name string
		call string
		pkgs []string
		want []string
	}{
		{"a package qualifier picks its directory", "store.Load()", []string{"store", "cache", "disk"}, []string{"store/f.go:3-5 definition of Load"}},
		{"two candidates are both shown", "x.Load()", []string{"store", "cache"}, []string{"cache/f.go:3-5 definition of Load", "store/f.go:3-5 definition of Load"}},
		{"more candidates are too ambiguous", "x.Load()", []string{"store", "cache", "disk"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := fakeRepo{"app/app.go": caller(tt.call)}
			for _, p := range tt.pkgs {
				repo[p+"/f.go"] = load(p)
			}
			got := findRelated(t, repo, addDiff(repo, "app/app.go", 4, 4), RelatedOptions{})
			assert.Equal(t, tt.want, labels(got))
		})
	}
}

func TestRelated_Callers(t *testing.T) {
	storeLoad := lineOf(t, storeGo, "return s.data[key]")
	storeTest := "package store\n\nfunc TestLoad(t *testing.T) {\n\t(&Store{}).Load(\"k\")\n}\n"

	t.Run("production callers", func(t *testing.T) {
		repo := fakeRepo{"app/app.go": appGo, "store/store.go": storeGo, "store/store_test.go": storeTest}
		got := findRelated(t, repo, addDiff(repo, "store/store.go", storeLoad, storeLoad), RelatedOptions{})
		assert.Equal(t, []string{"app/app.go:5-9 Run calls Load"}, labels(got))
	})

	t.Run("a test when nothing else calls it", func(t *testing.T) {
		repo := fakeRepo{"store/store.go": storeGo, "store/store_test.go": storeTest}
		got := findRelated(t, repo, addDiff(repo, "store/store.go", storeLoad, storeLoad), RelatedOptions{})
		assert.Equal(t, []string{"store/store_test.go:3-5 TestLoad calls Load"}, labels(got))
	})

	t.Run("too many call sites", func(t *testing.T) {
		repo := fakeRepo{"store/store.go": storeGo}
		for i := range maxCallSites + 1 {
			repo[fmt.Sprintf("app/f%d.go", i)] = fmt.Sprintf("package app\n\nfunc F%d(s *store.Store) {\n\ts.Load(\"k\")\n}\n", i)
		}
		got := findRelated(t, repo, addDiff(repo, "store/store.go", storeLoad, storeLoad), RelatedOptions{})
		assert.Empty(t, got)
	})

	t.Run("a long caller shows the lines around the call", func(t *testing.T) {
		long := "package app\n\nfunc Long(s *store.Store) {\n" + strings.Repeat("\t_ = 1\n", 30) + "\ts.Load(\"k\")\n" + strings.Repeat("\t_ = 2\n", 30) + "}\n"
		repo := fakeRepo{"store/store.go": storeGo, "app/long.go": long}
		got := findRelated(t, repo, addDiff(repo, "store/store.go", storeLoad, storeLoad), RelatedOptions{})
		call := lineOf(t, long, "s.Load")
		assert.Equal(t, []string{fmt.Sprintf("app/long.go:%d-%d Long calls Load; lines around the call, Long is lines 3-65", call-callerWindow, call+callerWindow)}, labels(got))
	})
}

func TestRelated_LongDefinitionIsCut(t *testing.T) {
	long := "package app\n\nfunc helper() {\n" + strings.Repeat("\t_ = 1\n", 80) + "}\n"
	repo := fakeRepo{"app/app.go": "package app\n\nfunc Run() {\n\thelper()\n}\n", "app/helper.go": long}

	got := findRelated(t, repo, addDiff(repo, "app/app.go", 4, 4), RelatedOptions{})

	require.Len(t, got, 1)
	assert.Equal(t, fmt.Sprintf("app/helper.go:3-%d definition of helper (first %d of 82 lines)", 2+maxDefinitionLines, maxDefinitionLines), labels(got)[0])
	assert.Len(t, got[0].Lines, maxDefinitionLines)
}

func TestRelated_Budget(t *testing.T) {
	repo := fakeRepo{"app/app.go": appGo, "app/helper.go": helperGo, "store/store.go": storeGo}
	run := lineOf(t, appGo, "func Run")

	// Load (6 lines) doesn't fit, helper (3) does.
	got := findRelated(t, repo, addDiff(repo, "app/app.go", run, run+4), RelatedOptions{MaxLines: 5})

	assert.Equal(t, []string{"app/helper.go:3-5 definition of helper"}, labels(got))
}

func TestRelated_Skip(t *testing.T) {
	repo := fakeRepo{"app/app.go": appGo, "app/helper.go": helperGo, "store/store.go": storeGo}
	run := lineOf(t, appGo, "func Run")

	got := findRelated(t, repo, addDiff(repo, "app/app.go", run, run+4), RelatedOptions{Skip: map[string]bool{"store/store.go": true}})

	assert.Equal(t, []string{"app/helper.go:3-5 definition of helper"}, labels(got))
}

// A definition the diff changes is shown by the diff itself.
func TestRelated_DefinitionInTheDiff(t *testing.T) {
	repo := fakeRepo{"app/app.go": appGo, "app/helper.go": helperGo}
	diff := addDiff(repo, "app/app.go", lineOf(t, appGo, "helper(v, n)"), lineOf(t, appGo, "helper(v, n)")) +
		addDiff(repo, "app/helper.go", 4, 4)

	assert.Empty(t, findRelated(t, repo, diff, RelatedOptions{}))
}

func TestRelated_Focus(t *testing.T) {
	repo := fakeRepo{"app/app.go": appGo, "app/helper.go": helperGo, "store/store.go": storeGo}
	// The diff only touches helper's body; the finding is on Run.
	diff := addDiff(repo, "app/helper.go", 4, 4)
	run := lineOf(t, appGo, "return helper")

	got := findRelated(t, repo, diff, RelatedOptions{Focus: map[string][]int{"app/app.go": {run}}})

	// Every call in Run, not just the focused line; helper is in the diff.
	assert.Equal(t, []string{"store/store.go:8-13 definition of Load"}, labels(got))
}

func TestRelated_FocusCallers(t *testing.T) {
	repo := fakeRepo{"app/app.go": appGo, "app/helper.go": helperGo, "store/store.go": storeGo}
	diff := addDiff(repo, "app/helper.go", 4, 4)

	got := findRelated(t, repo, diff, RelatedOptions{Focus: map[string][]int{"store/store.go": {lineOf(t, storeGo, "return s.data")}}})

	assert.Equal(t, []string{"app/app.go:5-9 Run calls Load"}, labels(got))
}

func TestRelated_OtherLanguages(t *testing.T) {
	tests := []struct {
		name   string
		repo   fakeRepo
		file   string
		change string
		want   []string
	}{
		{
			name: "typescript",
			repo: fakeRepo{
				"src/app.ts":  "import { format } from './fmt';\n\nexport function run(x: string) {\n  return format(x);\n}\n",
				"src/fmt.ts":  "export const format = (s: string) => {\n  return s.trim();\n};\n",
				"src/main.ts": "import { run } from './app';\n\nfunction main() {\n  run('x');\n}\n",
			},
			file: "src/app.ts", change: "return format(x)",
			want: []string{"src/fmt.ts:1-3 definition of format", "src/main.ts:3-5 main calls run"},
		},
		{
			name: "python",
			repo: fakeRepo{
				"app/run.py":  "from app.fmt import fmt\n\n\ndef run(x):\n    return fmt(x)\n",
				"app/fmt.py":  "def fmt(s):\n    return s.strip()\n",
				"app/main.py": "from app.run import run\n\n\ndef main():\n    run('x')\n",
			},
			file: "app/run.py", change: "return fmt(x)",
			want: []string{"app/fmt.py:1-2 definition of fmt", "app/main.py:4-5 main calls run"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := lineOf(t, tt.repo[tt.file], tt.change)
			got := findRelated(t, tt.repo, addDiff(tt.repo, tt.file, line, line), RelatedOptions{})
			assert.Equal(t, tt.want, labels(got))
		})
	}
}

func TestRelated_UnreadableAndUnsupportedFiles(t *testing.T) {
	repo := fakeRepo{"notes.txt": "helper\n", "app/helper.go": helperGo}
	diff := "diff --git a/missing.go b/missing.go\n--- a/missing.go\n+++ b/missing.go\n@@ -1,0 +1 @@\n+helper()\n" +
		addDiff(repo, "notes.txt", 1, 1)

	assert.Empty(t, findRelated(t, repo, diff, RelatedOptions{}))
}

func TestFormatRelated(t *testing.T) {
	assert.Empty(t, FormatRelated(nil))

	got := FormatRelated([]Piece{{Path: "a.go", Start: 7, End: 8, Label: "definition of F", Lines: []string{"func F() {", "}"}}})

	assert.True(t, strings.HasPrefix(got, "## Related code\n\n"), got)
	assert.Contains(t, got, "\n### a.go:7-8 (definition of F)\n\n7\tfunc F() {\n8\t}\n")
}

func TestRelated_SearchesAllNamesAtOnce(t *testing.T) {
	repo := fakeRepo{"app/app.go": appGo, "app/helper.go": helperGo, "store/store.go": storeGo}
	run := lineOf(t, appGo, "func Run")
	batches := 0

	got := Related(t.Context(), addDiff(repo, "app/app.go", run, run+4), batchGrep{repo, &batches}, RelatedOptions{MaxLines: 1000})

	assert.Equal(t, []string{"store/store.go:8-13 definition of Load", "app/helper.go:3-5 definition of helper"}, labels(got))
	assert.Equal(t, 1, batches)
}

// batchGrep answers GrepWords from the fake repo and fails single-word
// searches, so the test shows nothing falls back to them.
type batchGrep struct {
	repo    fakeRepo
	batches *int
}

func (b batchGrep) NewSideFile(ctx context.Context, p string) ([]byte, error) {
	return b.repo.NewSideFile(ctx, p)
}

func (b batchGrep) RepoFile(ctx context.Context, p string) ([]byte, error) {
	return b.repo.RepoFile(ctx, p)
}

func (b batchGrep) Grep(context.Context, string, ...string) ([]string, error) {
	return nil, errors.New("searched one word at a time")
}

func (b batchGrep) GrepWords(ctx context.Context, words []string, globs ...string) (map[string][]string, error) {
	*b.batches++
	out := map[string][]string{}
	for _, w := range words {
		if paths, _ := b.repo.Grep(ctx, w, globs...); len(paths) > 0 {
			out[w] = paths
		}
	}
	return out, nil
}

func TestRelated_TooCommonNames(t *testing.T) {
	repo := fakeRepo{"app/app.go": "package app\n\nfunc Run() {\n\thelper()\n}\n", "app/helper.go": helperGo}
	for i := range maxGrepFiles {
		repo[fmt.Sprintf("other/f%d.go", i)] = "package other\n\n// helper\n"
	}

	assert.Empty(t, findRelated(t, repo, addDiff(repo, "app/app.go", 4, 4), RelatedOptions{}))
}
