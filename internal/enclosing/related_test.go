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

	"github.com/ionut-t/bark/v2/internal/git"
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

	// Types first: the changed lines use s, a *store.Store. len is a
	// builtin.
	assert.Equal(t, []string{
		"store/store.go:3-5 definition of Store",
		"store/store.go:8-13 definition of Load",
		"app/helper.go:3-5 definition of helper",
	}, labels(got))
	assert.Equal(t, strings.Split(strings.TrimSpace(helperGo), "\n")[2:5], got[2].Lines)
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

func TestRelated_ImportedPackageCalls(t *testing.T) {
	// time.Since and strings.Cut name packages outside the repository, so
	// the repository's Since and Cut methods aren't theirs; a call on a
	// variable, or on a package of the repository, is still looked up.
	const reviewGo = "package review\n\nimport (\n\t\"strings\"\n\t\"time\"\n\n\t\"example.com/m/store\"\n)\n\n" +
		"func Run(d *Diff, start time.Time) {\n\t_ = time.Since(start)\n\t_, _, _ = strings.Cut(\"a.b\", \".\")\n\t_ = d.Grep(\"x\")\n\t_ = store.Open()\n}\n"
	const diffGo = "package review\n\ntype Diff struct{}\n\nfunc (d *Diff) Grep(word string) []string {\n\treturn nil\n}\n"
	const paramsGo = "package params\n\ntype Params struct{}\n\nfunc (p Params) Since() string {\n\treturn \"\"\n}\n"
	const rulesGo = "package rules\n\ntype Source struct{}\n\nfunc (s Source) Cut() {\n}\n"
	const storeGo = "package store\n\nfunc Open() int {\n\treturn 1\n}\n"
	repo := fakeRepo{
		"go.mod":           "module example.com/m\n",
		"review/review.go": reviewGo,
		"review/diff.go":   diffGo,
		"params/params.go": paramsGo,
		"rules/rules.go":   rulesGo,
		"store/store.go":   storeGo,
	}
	first, last := lineOf(t, reviewGo, "time.Since"), lineOf(t, reviewGo, "store.Open")

	got := findRelated(t, repo, addDiff(repo, "review/review.go", first, last), RelatedOptions{})

	assert.Equal(t, []string{
		"review/diff.go:3-3 definition of Diff",
		"review/diff.go:5-7 definition of Grep",
		"store/store.go:3-5 definition of Open",
	}, labels(got))
}

func TestRelated_Callers(t *testing.T) {
	storeLoad := lineOf(t, storeGo, "return s.data[key]")
	storeTest := "package store\n\nfunc TestLoad(t *testing.T) {\n\t(&Store{}).Load(\"k\")\n}\n"

	t.Run("production callers", func(t *testing.T) {
		repo := fakeRepo{"app/app.go": appGo, "store/store.go": storeGo, "store/store_test.go": storeTest}
		got := findRelated(t, repo, addDiff(repo, "store/store.go", storeLoad, storeLoad), RelatedOptions{})
		// The change reads s.data, a field of Store.
		assert.Equal(t, []string{"store/store.go:3-5 definition of Store", "app/app.go:5-9 Run calls Load"}, labels(got))
	})

	t.Run("a test when nothing else calls it", func(t *testing.T) {
		repo := fakeRepo{"store/store.go": storeGo, "store/store_test.go": storeTest}
		got := findRelated(t, repo, addDiff(repo, "store/store.go", storeLoad, storeLoad), RelatedOptions{})
		assert.Equal(t, []string{"store/store.go:3-5 definition of Store", "store/store_test.go:3-5 TestLoad calls Load"}, labels(got))
	})

	t.Run("too many call sites", func(t *testing.T) {
		repo := fakeRepo{"store/store.go": storeGo}
		for i := range maxCallSites + 1 {
			repo[fmt.Sprintf("app/f%d.go", i)] = fmt.Sprintf("package app\n\nfunc F%d(s *store.Store) {\n\ts.Load(\"k\")\n}\n", i)
		}
		got := findRelated(t, repo, addDiff(repo, "store/store.go", storeLoad, storeLoad), RelatedOptions{})
		assert.Equal(t, []string{"store/store.go:3-5 definition of Store"}, labels(got))
	})

	t.Run("a long caller shows the lines around the call", func(t *testing.T) {
		long := "package app\n\nfunc Long(s *store.Store) {\n" + strings.Repeat("\t_ = 1\n", 30) + "\ts.Load(\"k\")\n" + strings.Repeat("\t_ = 2\n", 30) + "}\n"
		repo := fakeRepo{"store/store.go": storeGo, "app/long.go": long}
		got := findRelated(t, repo, addDiff(repo, "store/store.go", storeLoad, storeLoad), RelatedOptions{})
		call := lineOf(t, long, "s.Load")
		assert.Equal(t, []string{"store/store.go:3-5 definition of Store", fmt.Sprintf("app/long.go:%d-%d Long calls Load; lines around the call, Long is lines 3-65", call-callerWindow, call+callerWindow)}, labels(got))
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

	// Store (3 lines) fits, Load (6) doesn't, helper (3) does.
	got := findRelated(t, repo, addDiff(repo, "app/app.go", run, run+4), RelatedOptions{MaxLines: 6})

	assert.Equal(t, []string{"store/store.go:3-5 definition of Store", "app/helper.go:3-5 definition of helper"}, labels(got))
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

	// Every call and type in Run, not just the focused line; helper is in
	// the diff.
	assert.Equal(t, []string{"store/store.go:3-5 definition of Store", "store/store.go:8-13 definition of Load"}, labels(got))
}

func TestRelated_FocusCallers(t *testing.T) {
	repo := fakeRepo{"app/app.go": appGo, "app/helper.go": helperGo, "store/store.go": storeGo}
	diff := addDiff(repo, "app/helper.go", 4, 4)

	got := findRelated(t, repo, diff, RelatedOptions{Focus: map[string][]int{"store/store.go": {lineOf(t, storeGo, "return s.data")}}})

	// The focused line reads s.data, a field of Store.
	assert.Equal(t, []string{"store/store.go:3-5 definition of Store", "app/app.go:5-9 Run calls Load"}, labels(got))
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

// git.ReviewDiff must keep matching ignoreChecker, or Related silently
// treats .barkignore'd files like any other.
var _ ignoreChecker = git.ReviewDiff{}

// ignoringRepo is a fakeRepo whose .barkignore leaves out the directories
// in ignored.
type ignoringRepo struct {
	fakeRepo
	ignored []string
}

func (r ignoringRepo) IgnoredFiles(context.Context) func(string) bool {
	return func(p string) bool { return slices.Contains(r.ignored, path.Dir(p)) }
}

func TestRelated_SameNamedDefinitionsInIgnoredFiles(t *testing.T) {
	load := func(pkg string) string {
		return "package " + pkg + "\n\nfunc Load() string {\n\treturn \"" + pkg + "\"\n}\n"
	}
	tests := []struct {
		name string
		call string
		pkgs []string
		want []string
	}{
		{"ignored files don't make a name ambiguous", "x.Load()", []string{"store", "gen1", "gen2"}, []string{"store/f.go:3-5 definition of Load"}},
		{"a qualifier still picks an ignored directory", "gen1.Load()", []string{"store", "gen1", "gen2"}, []string{"gen1/f.go:3-5 definition of Load"}},
		{"two candidates are both shown, ignored or not", "x.Load()", []string{"store", "gen1"}, []string{"gen1/f.go:3-5 definition of Load", "store/f.go:3-5 definition of Load"}},
		{"a definition only in an ignored file is shown", "x.Load()", []string{"gen1"}, []string{"gen1/f.go:3-5 definition of Load"}},
		{"too many ignored candidates stay ambiguous", "x.Load()", []string{"gen1", "gen2", "gen3"}, nil},
		{"too many others stay ambiguous", "x.Load()", []string{"store", "cache", "disk", "gen1"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := ignoringRepo{fakeRepo{"app/app.go": "package app\n\nfunc Run() {\n\t" + tt.call + "\n}\n"}, []string{"gen1", "gen2", "gen3"}}
			for _, p := range tt.pkgs {
				repo.fakeRepo[p+"/f.go"] = load(p)
			}
			got := Related(t.Context(), addDiff(repo.fakeRepo, "app/app.go", 4, 4), repo, RelatedOptions{MaxLines: 1000})
			assert.Equal(t, tt.want, labels(got))
		})
	}
}

func TestRelated_IgnoredFilesDontCountTowardsTooCommon(t *testing.T) {
	repo := ignoringRepo{fakeRepo{"app/app.go": "package app\n\nfunc Run() {\n\thelper()\n}\n", "app/helper.go": helperGo}, []string{"gen"}}
	for i := range maxGrepFiles {
		repo.fakeRepo[fmt.Sprintf("gen/f%d.go", i)] = "package gen\n\n// helper\n"
	}

	got := Related(t.Context(), addDiff(repo.fakeRepo, "app/app.go", 4, 4), repo, RelatedOptions{MaxLines: 1000})
	assert.Equal(t, []string{"app/helper.go:3-5 definition of helper"}, labels(got))
}

func TestRelated_SearchesAllNamesAtOnce(t *testing.T) {
	repo := fakeRepo{"app/app.go": appGo, "app/helper.go": helperGo, "store/store.go": storeGo}
	run := lineOf(t, appGo, "func Run")
	batches := 0

	got := Related(t.Context(), addDiff(repo, "app/app.go", run, run+4), batchGrep{repo, &batches}, RelatedOptions{MaxLines: 1000})

	assert.Equal(t, []string{"store/store.go:3-5 definition of Store", "store/store.go:8-13 definition of Load", "app/helper.go:3-5 definition of helper"}, labels(got))
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

func TestRelated_MethodSignaturesAreNotCallers(t *testing.T) {
	tests := []struct {
		name   string
		repo   fakeRepo
		file   string
		change string
		want   []string
	}{
		{
			name: "go interface",
			repo: fakeRepo{
				"app/app.go":     appGo,
				"store/store.go": storeGo,
				"app/loader.go":  "package app\n\ntype Loader interface {\n\tLoad(key string) string\n}\n",
			},
			file: "store/store.go", change: "return s.data[key]",
			want: []string{"store/store.go:3-5 definition of Store", "app/app.go:5-9 Run calls Load"},
		},
		{
			name: "typescript interface",
			repo: fakeRepo{
				"src/app.ts":    "export function run(x: string) {\n  return x.trim();\n}\n",
				"src/main.ts":   "import { run } from './app';\n\nfunction main() {\n  run('x');\n}\n",
				"src/runner.ts": "export interface Runner {\n  run(x: string): string;\n}\n",
			},
			file: "src/app.ts", change: "return x.trim()",
			want: []string{"src/main.ts:3-5 main calls run"},
		},
		{
			name: "rust trait",
			repo: fakeRepo{
				"src/app.rs":  "pub fn run(x: &str) -> String {\n    x.trim().to_owned()\n}\n",
				"src/main.rs": "fn main() {\n    run(\"x\");\n}\n",
				"src/lib.rs":  "pub trait Runner {\n    fn run(x: &str) -> String;\n}\n",
			},
			file: "src/app.rs", change: "x.trim()",
			want: []string{"src/main.rs:1-3 main calls run"},
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

func TestRelated_GoFunctionCallers(t *testing.T) {
	const rootGo = "package cmd\n\nfunc ExitCode(err error) int {\n\treturn 1\n}\n"
	change := lineOf(t, rootGo, "return 1")

	tests := []struct {
		name   string
		caller string // a file calling something named ExitCode
		path   string
		want   []string
	}{
		{
			name:   "unqualified in the same package",
			path:   "cmd/run.go",
			caller: "package cmd\n\nfunc Run() int {\n\treturn ExitCode(nil)\n}\n",
			want:   []string{"cmd/run.go:3-5 Run calls ExitCode"},
		},
		{
			name:   "qualified with the package name",
			path:   "main.go",
			caller: "package main\n\nimport \"example.com/m/cmd\"\n\nfunc main() {\n\tos.Exit(cmd.ExitCode(nil))\n}\n",
			want:   []string{"main.go:5-7 main calls ExitCode"},
		},
		{
			name:   "qualified with an import alias",
			path:   "main.go",
			caller: "package main\n\nimport c \"example.com/m/cmd\"\n\nfunc main() {\n\tos.Exit(c.ExitCode(nil))\n}\n",
			want:   []string{"main.go:5-7 main calls ExitCode"},
		},
		{
			name:   "unqualified through a dot import",
			path:   "main.go",
			caller: "package main\n\nimport . \"example.com/m/cmd\"\n\nfunc main() {\n\tos.Exit(ExitCode(nil))\n}\n",
			want:   []string{"main.go:5-7 main calls ExitCode"},
		},
		{
			name:   "an alias for another package",
			path:   "main.go",
			caller: "package main\n\nimport cmd \"example.com/m/other\"\n\nfunc main() {\n\tos.Exit(cmd.ExitCode(nil))\n}\n",
		},
		{
			name:   "a method of the same name",
			path:   "git/git.go",
			caller: "package git\n\nfunc Run() int {\n\tvar exitErr *exec.ExitError\n\treturn exitErr.ExitCode()\n}\n",
		},
		{
			name:   "a method on a call's result",
			path:   "git/git.go",
			caller: "package git\n\nfunc Run() int {\n\treturn wait().ExitCode()\n}\n",
		},
		{
			name:   "unqualified in another package",
			path:   "git/git.go",
			caller: "package git\n\nfunc Run() int {\n\tExitCode := func() int { return 0 }\n\treturn ExitCode()\n}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := fakeRepo{"cmd/root.go": rootGo, tt.path: tt.caller}
			got := findRelated(t, repo, addDiff(repo, "cmd/root.go", change, change), RelatedOptions{})
			assert.Equal(t, tt.want, labels(got))
		})
	}
}

func TestRelated_GoFunctionCallersByImport(t *testing.T) {
	const rootGo = "package cmd\n\nfunc ExitCode(err error) int {\n\treturn 1\n}\n"
	const libGo = "package lib\n\nfunc Do() int {\n\treturn 1\n}\n"

	tests := []struct {
		name   string
		repo   fakeRepo
		change string // the changed file
		want   []string
	}{
		{
			name: "an unaliased import of another package with the same name",
			repo: fakeRepo{
				"go.mod":      "module example.com/m\n",
				"cmd/root.go": rootGo,
				"main.go":     "package main\n\nimport \"github.com/other/cmd\"\n\nfunc main() {\n\tos.Exit(cmd.ExitCode(nil))\n}\n",
			},
			change: "cmd/root.go",
		},
		{
			name: "an unaliased import of the package",
			repo: fakeRepo{
				"go.mod":      "module example.com/m\n",
				"cmd/root.go": rootGo,
				"main.go":     "package main\n\nimport \"example.com/m/cmd\"\n\nfunc main() {\n\tos.Exit(cmd.ExitCode(nil))\n}\n",
			},
			change: "cmd/root.go",
			want:   []string{"main.go:5-7 main calls ExitCode"},
		},
		{
			name: "a local variable named like the package",
			repo: fakeRepo{
				"go.mod":      "module example.com/m\n",
				"cmd/root.go": rootGo,
				"run/run.go":  "package run\n\nfunc Run() int {\n\tcmd := start()\n\treturn cmd.ExitCode()\n}\n",
			},
			change: "cmd/root.go",
		},
		{
			name: "a package named unlike its directory",
			repo: fakeRepo{
				"go.mod":        "module example.com/m\n",
				"genai/root.go": strings.Replace(rootGo, "package cmd", "package gemini", 1),
				"main.go":       "package main\n\nimport \"example.com/m/genai\"\n\nfunc main() {\n\tos.Exit(gemini.ExitCode(nil))\n}\n",
			},
			change: "genai/root.go",
			want:   []string{"main.go:5-7 main calls ExitCode"},
		},
		{
			name: "several dot imports",
			repo: fakeRepo{
				"go.mod":      "module example.com/m\n",
				"cmd/root.go": rootGo,
				"main.go":     "package main\n\nimport (\n\t. \"example.com/m/cmd\"\n\t. \"example.com/m/other\"\n)\n\nfunc main() {\n\tos.Exit(ExitCode(nil))\n}\n",
			},
			change: "cmd/root.go",
			want:   []string{"main.go:8-10 main calls ExitCode"},
		},
		{
			name: "a method on an index or call that mentions the package",
			repo: fakeRepo{
				"go.mod":      "module example.com/m\n",
				"cmd/root.go": rootGo,
				"main.go":     "package main\n\nimport \"example.com/m/cmd\"\n\nfunc main() {\n\t_ = values[cmd].ExitCode()\n\t_ = fn(cmd).ExitCode()\n\t_ = fn(cmd) .ExitCode()\n}\n",
			},
			change: "cmd/root.go",
		},
		{
			name: "a package in a nested module",
			repo: fakeRepo{
				"go.mod":            "module example.com/m\n",
				"tools/go.mod":      "module example.com/tools\n",
				"tools/cmd/root.go": rootGo,
				"tools/main.go":     "package main\n\nimport \"example.com/tools/cmd\"\n\nfunc main() {\n\tos.Exit(cmd.ExitCode(nil))\n}\n",
			},
			change: "tools/cmd/root.go",
			want:   []string{"tools/main.go:5-7 main calls ExitCode"},
		},
		{
			name: "the root package through an alias",
			repo: fakeRepo{
				"go.mod":   "module example.com/lib\n\ngo 1.27\n",
				"lib.go":   libGo,
				"sub/x.go": "package sub\n\nimport l \"example.com/lib\"\n\nfunc X() int {\n\treturn l.Do()\n}\n",
			},
			change: "lib.go",
			want:   []string{"sub/x.go:5-7 X calls Do"},
		},
		{
			name: "the root package without an alias",
			repo: fakeRepo{
				"go.mod":   "module example.com/lib\n\ngo 1.27\n",
				"lib.go":   libGo,
				"sub/x.go": "package sub\n\nimport \"example.com/lib\"\n\nfunc X() int {\n\treturn lib.Do()\n}\n",
			},
			change: "lib.go",
			want:   []string{"sub/x.go:5-7 X calls Do"},
		},
		{
			name: "the root package without a go.mod",
			repo: fakeRepo{
				"lib.go":   libGo,
				"sub/x.go": "package sub\n\nimport t \"example.com/other/tasks\"\n\nfunc X() int {\n\treturn t.Do()\n}\n",
			},
			change: "lib.go",
		},
		{
			name: "another package's function named like a root one",
			repo: fakeRepo{
				"go.mod":   "module example.com/lib\n\ngo 1.27\n",
				"lib.go":   libGo,
				"sub/x.go": "package sub\n\nimport \"example.com/other/tasks\"\n\nfunc X() int {\n\treturn tasks.Do()\n}\n",
			},
			change: "lib.go",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := lineOf(t, tt.repo[tt.change], "return 1")
			got := findRelated(t, tt.repo, addDiff(tt.repo, tt.change, line, line), RelatedOptions{})
			assert.Equal(t, tt.want, labels(got))
		})
	}
}

// A line of a long type that declares two names the change uses is shown
// once, labelled with both names, the same on every run.
func TestRelated_MembersOnOneLine(t *testing.T) {
	var b strings.Builder
	b.WriteString("package tui\n\ntype Model struct {\n\twidth, height int\n")
	for i := range 70 {
		fmt.Fprintf(&b, "\tfield%d string\n", i)
	}
	b.WriteString("}\n")
	repo := fakeRepo{
		"tui/model.go": b.String(),
		"tui/view.go":  "package tui\n\nfunc (m Model) View() int {\n\treturn m.width * m.height\n}\n",
	}

	// The members are a map, so one run may pass by chance.
	for range 20 {
		got := relatedTo(t, repo, "tui/view.go", "m.width * m.height")
		require.Equal(t, []string{"tui/model.go:4-4 height, width in Model, lines 3-75"}, got)
	}
}

// Members starting on one line and both cut to maxDefinitionLines end up
// with the same range; their order mustn't depend on the map.
func TestDefinitionPieces_SameCutRange(t *testing.T) {
	d := decl{path: "a.go", name: "T", start: 1, end: 400, members: map[string][]member{
		"a": {{span: span{10, 200}}},
		"b": {{span: span{10, 300}}},
	}}
	for range 20 {
		got := labels(definitionPieces(d, map[string]bool{"a": true, "b": true}))
		require.Equal(t, []string{"a.go:10-69 a in T, lines 1-400", "a.go:10-69 b in T, lines 1-400"}, got)
	}
}
