package prreview

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// diff changes a.go in two hunks (new lines 1-4 and 20-23) and deletes
// gone.go.
const diff = `diff --git a/a.go b/a.go
index 1111111..2222222 100644
--- a/a.go
+++ b/a.go
@@ -1,3 +1,4 @@
 package a
-var x = 1
+var x = 2
+var y = 3

@@ -19,3 +20,4 @@ func f() {
 	a()
+	b()
 	c()
 }
diff --git a/gone.go b/gone.go
deleted file mode 100644
--- a/gone.go
+++ /dev/null
@@ -1,2 +0,0 @@
-package gone
-var z = 1
`

func TestCommentableLines(t *testing.T) {
	t.Parallel()

	got := commentableLines(diff)
	assert.Equal(t, map[string]map[int]diffLine{
		"a.go": {
			1: {1, "package a"}, 2: {1, "var x = 2"}, 3: {1, "var y = 3"}, 4: {1, ""},
			20: {2, "\ta()"}, 21: {2, "\tb()"}, 22: {2, "\tc()"}, 23: {2, "}"},
		},
	}, got)
}

func finding(line, endLine int, suggestion string) findings.Event {
	return fix(line, endLine, "", suggestion)
}

func fix(line, endLine int, original, suggestion string) findings.Event {
	return findings.Event{Finding: &findings.Finding{
		File: "a.go", Line: line, EndLine: endLine, Severity: findings.High, Category: "bug",
		Title: "T", Body: "B.", Original: original, Suggestion: suggestion, Remark: "R.",
	}}
}

func suggestionBody(code string) string {
	return "**[HIGH · bug] T**\n\nB.\n\n```suggestion\n" + code + "\n```\n\n> _R._"
}

func plainCodeBody(code string) string {
	return "**[HIGH · bug] T**\n\nB.\n\n```\n" + code + "\n```\n\n> _R._"
}

func TestBuild(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		event     findings.Event
		want      []Comment
		inBody    string
		notInBody string
	}{
		{
			name:  "suggestion on the original line",
			event: fix(2, 0, "var x = 2", "var x = 1"),
			want:  []Comment{{Path: "a.go", Line: 2, Side: "RIGHT", Body: suggestionBody("var x = 1")}},
		},
		{
			name:  "suggestion over a range",
			event: fix(2, 3, "var x = 2\nvar y = 3\n", "var x = 1\nvar y = 1\n"),
			want: []Comment{{Path: "a.go", StartLine: 2, StartSide: "RIGHT", Line: 3, Side: "RIGHT",
				Body: suggestionBody("var x = 1\nvar y = 1")}},
		},
		{
			name:  "line number off by one moves to the original",
			event: fix(3, 0, "var x = 2", "var x = 1"),
			want:  []Comment{{Path: "a.go", Line: 2, Side: "RIGHT", Body: suggestionBody("var x = 1")}},
		},
		{
			name:  "original matched despite indentation",
			event: fix(22, 0, "b()", "\tb(ctx)"),
			want:  []Comment{{Path: "a.go", Line: 21, Side: "RIGHT", Body: suggestionBody("\tb(ctx)")}},
		},
		{
			name:  "original found on a line outside the given range is used",
			event: fix(10, 0, "}", "\treturn\n}"),
			want:  []Comment{{Path: "a.go", Line: 23, Side: "RIGHT", Body: suggestionBody("\treturn\n}")}},
		},
		{
			name:  "original not in the diff shows plain code",
			event: fix(2, 0, "var q = 9", "var x = 1"),
			want:  []Comment{{Path: "a.go", Line: 2, Side: "RIGHT", Body: plainCodeBody("var x = 1")}},
		},
		{
			name:  "original across hunks shows plain code",
			event: fix(4, 0, "\n\ta()", "x"),
			want:  []Comment{{Path: "a.go", Line: 4, Side: "RIGHT", Body: plainCodeBody("x")}},
		},
		{
			name:  "suggestion without original shows plain code",
			event: finding(2, 3, "x"),
			want: []Comment{{Path: "a.go", StartLine: 2, StartSide: "RIGHT", Line: 3, Side: "RIGHT",
				Body: plainCodeBody("x")}},
		},
		{
			name:  "range across hunks keeps the first line",
			event: finding(4, 20, ""),
			want:  []Comment{{Path: "a.go", Line: 4, Side: "RIGHT", Body: "**[HIGH · bug] T**\n\nB.\n\n> _R._"}},
		},
		{
			name:  "suggestion containing a fence",
			event: fix(21, 0, "b()", "s := \"```\""),
			want: []Comment{{Path: "a.go", Line: 21, Side: "RIGHT",
				Body: "**[HIGH · bug] T**\n\nB.\n\n````suggestion\n\ts := \"```\"\n````\n\n> _R._"}},
		},
		{
			name:   "line outside the diff goes in the body",
			event:  finding(10, 0, "x"),
			inBody: "### [HIGH · bug] T\n\n`a.go:10`",
		},
		{
			name:   "no line goes in the body",
			event:  finding(0, 0, ""),
			inBody: "### [HIGH · bug] T",
		},
		{
			name: "file not in the diff goes in the body",
			event: findings.Event{Finding: &findings.Finding{
				File: "b.go", Line: 2, Severity: findings.Low, Title: "Other"}},
			inBody: "### [LOW] Other",
		},
		{
			name: "deleted file goes in the body",
			event: findings.Event{Finding: &findings.Finding{
				File: "gone.go", Line: 1, Severity: findings.Low, Title: "Gone"}},
			inBody: "### [LOW] Gone",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			events := []findings.Event{{Intro: "Hello."}, tt.event, {Verdict: "Bye."}}
			got := Build(events, diff)

			assert.Equal(t, "COMMENT", got.Event)
			assert.Equal(t, tt.want, got.Comments)
			assert.True(t, strings.HasPrefix(got.Body, Marker+"\n## Bark AI Code Review\n\nHello.\n\n"), got.Body)
			assert.True(t, strings.HasSuffix(got.Body, "## Verdict\n\nBye.\n"), got.Body)
			if tt.inBody != "" {
				assert.Contains(t, got.Body, tt.inBody)
			} else {
				assert.NotContains(t, got.Body, "### [")
			}
		})
	}
}

func TestBuildWithoutDiffPutsEverythingInBody(t *testing.T) {
	t.Parallel()

	got := Build([]findings.Event{finding(2, 0, "x")}, "")
	assert.Empty(t, got.Comments)
	assert.Contains(t, got.Body, "### [HIGH · bug] T\n\n`a.go:2`")
}

func TestReviewsEndpoint(t *testing.T) {
	t.Parallel()

	got, err := reviewsEndpoint("https://github.com/ionut-t/bark/pull/12")
	require.NoError(t, err)
	assert.Equal(t, "repos/ionut-t/bark/pulls/12/reviews", got)

	_, err = reviewsEndpoint("https://github.com/ionut-t/bark")
	assert.Error(t, err)
}

// fakeGH answers gh pr view and records the reviews posted; postErrs are
// returned by successive gh api calls.
type fakeGH struct {
	posted   []Review
	postErrs []error
}

func (f *fakeGH) run(_ context.Context, stdin []byte, args ...string) ([]byte, error) {
	switch args[0] {
	case "pr":
		return []byte(`{"url":"https://github.com/o/r/pull/7","headRefOid":"abc123"}`), nil
	case "api":
		if args[3] != "repos/o/r/pulls/7/reviews" {
			return nil, errors.New("wrong endpoint " + args[3])
		}
		var r Review
		if err := json.Unmarshal(stdin, &r); err != nil {
			return nil, err
		}
		f.posted = append(f.posted, r)
		if n := len(f.posted); n <= len(f.postErrs) && f.postErrs[n-1] != nil {
			return nil, f.postErrs[n-1]
		}
		return []byte(`{"html_url":"https://github.com/o/r/pull/7#pullrequestreview-1"}`), nil
	}
	return nil, errors.New("unexpected gh call")
}

func TestPost(t *testing.T) {
	t.Parallel()

	events := []findings.Event{finding(2, 0, "x"), finding(10, 0, "")}
	gh := &fakeGH{}

	got, err := post(context.Background(), gh.run, "7", events, diff)
	require.NoError(t, err)
	assert.Equal(t, Result{URL: "https://github.com/o/r/pull/7#pullrequestreview-1", Inline: 1}, got)
	require.Len(t, gh.posted, 1)
	assert.Equal(t, "abc123", gh.posted[0].CommitID)
	assert.Len(t, gh.posted[0].Comments, 1)
}

func TestPostFallsBackToBody(t *testing.T) {
	t.Parallel()

	events := []findings.Event{finding(2, 0, "x")}
	gh := &fakeGH{postErrs: []error{errors.New("HTTP 422: Line could not be resolved")}}

	got, err := post(context.Background(), gh.run, "7", events, diff)
	require.NoError(t, err)
	assert.True(t, got.Fallback)
	assert.Zero(t, got.Inline)
	require.Len(t, gh.posted, 2)
	assert.Empty(t, gh.posted[1].Comments)
	assert.Contains(t, gh.posted[1].Body, "`a.go:2`")
}

func TestPostReportsBothErrors(t *testing.T) {
	t.Parallel()

	gh := &fakeGH{postErrs: []error{errors.New("first"), errors.New("second")}}

	_, err := post(context.Background(), gh.run, "7", []findings.Event{finding(2, 0, "")}, diff)
	require.Error(t, err)
	assert.ErrorContains(t, err, "first")
	assert.ErrorContains(t, err, "second")
}

// A finding that isn't valid JSON even after repair (here, cut off before
// its end) is shown in the body folded away as code, not as a raw line.
func TestBuildFoldsUnreadableFindings(t *testing.T) {
	t.Parallel()

	bad := `{"file":"a.go","line":2,"title":"T","original":"if msg != \"\" {`
	events := findings.Parse("{\"type\":\"intro\",\"text\":\"Hi.\"}\n" + bad + "\nsome prose\n{\"type\":\"verdict\",\"text\":\"Bye.\"}\n")

	got := Build(events, diff)
	assert.Contains(t, got.Body, "Hi.\n\n<details>\n<summary>A finding Bark couldn't read</summary>\n\n```json\n"+bad+"\n```\n\n</details>\n\nsome prose\n## Verdict")
	assert.Equal(t, 1, strings.Count(got.Body, bad))
}

func TestBuildFoldsUnreadableFindingContainingFence(t *testing.T) {
	t.Parallel()

	bad := `{"file":"a.go","suggestion":"` + "```" + `" x}`
	got := Build([]findings.Event{{Text: bad}}, diff)
	assert.Contains(t, got.Body, "````json\n"+bad+"\n````")
}

func TestFixIndent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		original   []string
		file       []string
		suggestion string
		want       string
		wantOK     bool
	}{
		{
			name:       "exact copy is unchanged",
			original:   []string{"\t\treturn x"},
			file:       []string{"\t\treturn x"},
			suggestion: "\t\treturn y\n",
			want:       "\t\treturn y\n",
			wantOK:     true,
		},
		{
			// PR #104, post.go:56.
			name:       "single line that lost its indentation",
			original:   []string{"return Result{}, err"},
			file:       []string{"\t\treturn Result{}, err"},
			suggestion: "return Result{}, fmt.Errorf(\"x: %w\", err)",
			want:       "\t\treturn Result{}, fmt.Errorf(\"x: %w\", err)",
			wantOK:     true,
		},
		{
			// PR #104, post.go:89: only the first line lost it, the rest
			// of the suggestion is already indented for the file.
			name:       "one original line, several suggestion lines",
			original:   []string{"_ = json.Unmarshal(out, &created)"},
			file:       []string{"\t_ = json.Unmarshal(out, &created)"},
			suggestion: "if err := json.Unmarshal(out, &created); err != nil {\n\t\treturn err\n\t}",
			wantOK:     false,
		},
		{
			name:       "original lost it but the suggestion kept it",
			original:   []string{"b()"},
			file:       []string{"\tb()"},
			suggestion: "\tb(ctx)",
			want:       "\tb(ctx)",
			wantOK:     true,
		},
		{
			name:       "every line lost the same indentation",
			original:   []string{"if x {", "\treturn", "}"},
			file:       []string{"\tif x {", "\t\treturn", "\t}"},
			suggestion: "if y {\n\treturn\n\n}",
			want:       "\tif y {\n\t\treturn\n\n\t}",
			wantOK:     true,
		},
		{
			name:       "only the first line lost it",
			original:   []string{"if x {", "\t\treturn", "\t}"},
			file:       []string{"\tif x {", "\t\treturn", "\t}"},
			suggestion: "if y {\n\t\treturn\n\t}",
			want:       "\tif y {\n\t\treturn\n\t}",
			wantOK:     true,
		},
		{
			name:       "lines lost different amounts",
			original:   []string{"if x {", "return", "\t}"},
			file:       []string{"\tif x {", "\t\treturn", "\t}"},
			suggestion: "if y {\nreturn\n\t}",
			wantOK:     false,
		},
		{
			name:       "original has more indentation than the file",
			original:   []string{"\t\t\tb()"},
			file:       []string{"\tb()"},
			suggestion: "\t\t\tb(ctx)",
			wantOK:     false,
		},
		{
			name:       "spaces instead of tabs",
			original:   []string{"  b()"},
			file:       []string{"\tb()"},
			suggestion: "  b(ctx)",
			wantOK:     false,
		},
		{
			name:       "suggestion indented differently from original",
			original:   []string{"b()"},
			file:       []string{"\t\tb()"},
			suggestion: "\tb(ctx)",
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := fixIndent(tt.original, tt.file, tt.suggestion)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// PR #104, prreview.go:135: the suggestion block posted there had lost its
// indentation; it must now be indented like the line it replaces.
func TestBuildRestoresIndentation(t *testing.T) {
	t.Parallel()

	got := Build([]findings.Event{fix(21, 0, "b()", "b(ctx)")}, diff)
	require.Len(t, got.Comments, 1)
	assert.Equal(t, suggestionBody("\tb(ctx)"), got.Comments[0].Body)

	got = Build([]findings.Event{fix(21, 0, "b()", "if b() {\n\t\tc()\n\t}")}, diff)
	require.Len(t, got.Comments, 1)
	assert.Equal(t, plainCodeBody("if b() {\n\t\tc()\n\t}"), got.Comments[0].Body)
}
