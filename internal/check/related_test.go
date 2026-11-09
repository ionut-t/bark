package check

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/enclosing"
	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/stretchr/testify/assert"
)

var logPiece = enclosing.Piece{
	Path: "log/log.go", Start: 10, End: 14, Label: "definition of Write",
	Lines: []string{"func (l *Log) Write(s string) {", "\tif l == nil {", "\t\treturn", "\t}", "}"},
}

func TestPromptIncludesRelatedCode(t *testing.T) {
	t.Parallel()

	in := input()
	in.Related = []enclosing.Piece{logPiece}
	got := Prompt(in, []findings.Finding{{File: "a.go", Line: 4, Title: "t", Body: "b"}})

	rel := strings.Index(got, "## Related code")
	assert.Positive(t, rel)
	assert.Less(t, strings.Index(got, "## Files"), rel)
	assert.Less(t, rel, strings.Index(got, "## Findings"))
	assert.Contains(t, got, "### log/log.go:10-14 (definition of Write)\n\n10\tfunc (l *Log) Write(s string) {\n11\t\tif l == nil {\n")

	assert.NotContains(t, Prompt(input(), nil), "## Related code")
}

func TestApplyEvidenceFromRelatedCode(t *testing.T) {
	t.Parallel()

	fs := []findings.Finding{{File: "a.go", Line: 4}}
	keep := map[int]Verdict{1: {ID: 1, Verdict: "keep", Evidence: "if l == nil {", Reason: "r"}}

	without := Apply(input(), fs, keep)
	assert.False(t, without[0].Kept, "evidence from code the check wasn't shown")

	in := input()
	in.Related = []enclosing.Piece{logPiece}
	assert.True(t, Apply(in, fs, keep)[0].Kept)
}

// repo is an in-memory enclosing.Repo.
type repo map[string]string

func (r repo) NewSideFile(_ context.Context, p string) ([]byte, error) {
	if c, ok := r[p]; ok {
		return []byte(c), nil
	}
	return nil, errors.New("no such file")
}

func (r repo) RepoFile(ctx context.Context, p string) ([]byte, error) { return r.NewSideFile(ctx, p) }

func (r repo) Grep(_ context.Context, word string, _ ...string) ([]string, error) {
	var out []string
	for p, c := range r {
		if strings.Contains(c, word) {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out, nil
}

// The check gets the code around the findings' lines from other files:
// here, the nil-safe method a "nil dereference" finding is about.
func TestRelated(t *testing.T) {
	t.Parallel()

	files := repo{
		"a.go":       "package a\n\nfunc run(l *log.Log) {\n\tl.Write(\"x\")\n}\n",
		"log/log.go": "package log\n\ntype Log struct{}\n\nfunc (l *Log) Write(s string) {\n\tif l == nil {\n\t\treturn\n\t}\n}\n",
		"b.go":       "package a\n\nfunc other() { run(nil) }\n",
	}
	diff := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -4,0 +4 @@\n+\tl.Write(\"x\")\n"

	tests := []struct {
		name  string
		fs    []findings.Finding
		shown map[string]string
		want  []string
	}{
		{"definitions and callers", []findings.Finding{{File: "a.go", Line: 4}}, nil, []string{"log/log.go definition of Write", "b.go other calls run"}},
		{"files already shown are left out", []findings.Finding{{File: "a.go", Line: 4}}, map[string]string{"log/log.go": ""}, []string{"b.go other calls run"}},
		{"findings without a line", []findings.Finding{{File: "a.go"}}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got []string
			for _, pc := range related(t.Context(), files, diff, tt.fs, tt.shown) {
				got = append(got, pc.Path+" "+pc.Label)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
