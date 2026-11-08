package check

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/debuglog"
	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const diff = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,3 +1,4 @@
     1  package a
     2
     3 +func f(m map[string]int) {
       -func g() {
     4 +	m["x"] = 1
`

const fileA = "package a\n\nfunc f(m map[string]int) {\n\tm[\"x\"] = 1\n}\n\nfunc h() { f(nil) }\n"

func input() Input {
	return Input{Diff: diff, Files: map[string]string{"a.go": fileA}}
}

func TestParseVerdicts(t *testing.T) {
	t.Parallel()

	out := "```json\n" +
		`{"id":1,"verdict":"KEEP","severity":" High ","suggestion":"Broken","evidence":"x","reason":"r1"}` + "\n" +
		"some prose\n" +
		`{"id":2,"verdict":"drop","reason":"first"}` + "\n" +
		`{"id":2,"verdict":"keep","evidence":"y","reason":"second"}` + "\n" +
		`{"id":0,"verdict":"drop"}` + "\n" +
		`{"id":3,"verdict":` + "\n" +
		"```"

	got := ParseVerdicts(out)

	assert.Equal(t, map[int]Verdict{
		1: {ID: 1, Verdict: "keep", Severity: "high", Suggestion: "broken", Evidence: "x", Reason: "r1"},
		2: {ID: 2, Verdict: "keep", Evidence: "y", Reason: "second"},
	}, got)
}

func TestApply(t *testing.T) {
	t.Parallel()

	finding := findings.Finding{File: "a.go", Line: 4, Severity: findings.High, Title: "t", Suggestion: "s", Original: "o"}

	tests := []struct {
		name    string
		verdict *Verdict
		want    Outcome
	}{
		{
			name:    "no verdict keeps the finding unchecked",
			verdict: nil,
			want:    Outcome{Finding: finding, Kept: true, Unchecked: true, Reason: "not checked"},
		},
		{
			name:    "drop",
			verdict: &Verdict{Verdict: "drop", Reason: "m is never nil"},
			want:    Outcome{Finding: finding, Reason: "m is never nil"},
		},
		{
			name:    "keep with evidence from the diff, other severity",
			verdict: &Verdict{Verdict: "keep", Severity: "low", Evidence: `m["x"] = 1`, Reason: "nil map"},
			want: Outcome{
				Finding: findings.Finding{File: "a.go", Line: 4, Severity: findings.Low, Title: "t", Suggestion: "s", Original: "o"},
				Kept:    true, Reason: "nil map",
			},
		},
		{
			name:    "keep with evidence from a removed line",
			verdict: &Verdict{Verdict: "keep", Evidence: "func g() {", Reason: "r"},
			want:    Outcome{Finding: finding, Kept: true, Reason: "r"},
		},
		{
			name: "evidence copied with line numbers, diff markers and other spacing",
			verdict: &Verdict{Verdict: "keep", Evidence: "3 +func f(m  map[string]int) {\n4:\tm[\"x\"] = 1\n...",
				Reason: "r"},
			want: Outcome{Finding: finding, Kept: true, Reason: "r"},
		},
		{
			name:    "evidence only in the full file",
			verdict: &Verdict{Verdict: "keep", Evidence: "f(nil)", Reason: "r"},
			want:    Outcome{Finding: finding, Kept: true, Reason: "r"},
		},
		{
			name:    "a lone brace isn't evidence",
			verdict: &Verdict{Verdict: "keep", Evidence: "}\n)", Reason: "r"},
			want:    Outcome{Finding: finding, Reason: "the check couldn't quote code that backs it"},
		},
		{
			name:    "evidence that isn't in the code",
			verdict: &Verdict{Verdict: "keep", Evidence: "m := map[string]int(nil)", Reason: "r"},
			want:    Outcome{Finding: finding, Reason: "the check couldn't quote code that backs it"},
		},
		{
			name:    "one line of the evidence isn't in the code",
			verdict: &Verdict{Verdict: "keep", Evidence: "m[\"x\"] = 1\nreturn nil", Reason: "r"},
			want:    Outcome{Finding: finding, Reason: "the check couldn't quote code that backs it"},
		},
		{
			name:    "no evidence",
			verdict: &Verdict{Verdict: "keep", Evidence: " \n...", Reason: "r"},
			want:    Outcome{Finding: finding, Reason: "the check couldn't quote code that backs it"},
		},
		{
			name:    "severity outside the scale is ignored",
			verdict: &Verdict{Verdict: "keep", Severity: "nuclear", Evidence: "package a", Reason: "r"},
			want:    Outcome{Finding: finding, Kept: true, Reason: "r"},
		},
		{
			name:    "broken suggestion is removed",
			verdict: &Verdict{Verdict: "keep", Suggestion: "broken", Evidence: "package a", Reason: "r"},
			want: Outcome{
				Finding: findings.Finding{File: "a.go", Line: 4, Severity: findings.High, Title: "t"},
				Kept:    true, Reason: "r",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			verdicts := map[int]Verdict{}
			if tt.verdict != nil {
				verdicts[1] = *tt.verdict
			}
			got := Apply(input(), []findings.Finding{finding}, verdicts)
			assert.Equal(t, []Outcome{tt.want}, got)
		})
	}
}

func TestPrompt(t *testing.T) {
	t.Parallel()

	in := input()
	in.Header = "## Go version\n\ngo.mod targets Go 1.27.\n"
	in.Files["unrelated.go"] = "package unrelated\n"
	fs := []findings.Finding{
		{File: "a.go", Line: 4, Severity: findings.High, Category: "bug", Title: "Nil map", Body: "m may be nil.", Remark: "Disgraceful."},
		{File: "gone.go", Severity: findings.Low, Title: "Deleted", Body: "b"},
	}

	got := Prompt(in, fs)

	assert.True(t, strings.HasPrefix(got, "## Go version\n\ngo.mod targets Go 1.27.\n\n## Diff\n\n"+diff))
	assert.Contains(t, got, "### a.go\n\n1\tpackage a\n2\t\n3\tfunc f(m map[string]int) {\n4\t\tm[\"x\"] = 1\n5\t}\n6\t\n7\tfunc h() { f(nil) }\n")
	assert.NotContains(t, got, "unrelated", "files no finding points to are left out")
	assert.NotContains(t, got, "### gone.go", "files that couldn't be read are left out")
	assert.Contains(t, got, "## Findings\n\n"+
		`1. {"file":"a.go","line":4,"severity":"high","category":"bug","title":"Nil map","body":"m may be nil."}`+"\n"+
		`2. {"file":"gone.go","severity":"low","title":"Deleted","body":"b"}`+"\n")
	assert.NotContains(t, got, "Disgraceful", "the persona's remark isn't checked")
}

func TestLoadFiles(t *testing.T) {
	t.Parallel()

	events := []findings.Event{
		{Intro: "hi"},
		{Finding: &findings.Finding{File: "a.go"}},
		{Finding: &findings.Finding{File: "a.go", Line: 2}},
		{Finding: &findings.Finding{File: "gone.go"}},
		{Finding: &findings.Finding{File: "image.png"}},
		{Finding: &findings.Finding{}},
	}
	var read []string
	got := LoadFiles(events, func(path string) ([]byte, error) {
		read = append(read, path)
		switch path {
		case "a.go":
			return []byte(fileA), nil
		case "image.png":
			return []byte("\x89PNG\x00\x01"), nil
		}
		return nil, os.ErrNotExist
	})

	assert.Equal(t, map[string]string{"a.go": fileA}, got)
	assert.Equal(t, []string{"a.go", "gone.go", "image.png"}, read, "each file is read once")
}

// fakeLLM answers Generate with output, or err, and records the calls.
type fakeLLM struct {
	output string
	err    error
	calls  *int
}

func (f fakeLLM) Stream(context.Context, string, string) (<-chan llm.Response, <-chan error) {
	panic("the check doesn't stream")
}

func (f fakeLLM) Generate(_ context.Context, system, prompt string) (llm.Response, error) {
	*f.calls++
	if f.err != nil {
		return llm.Response{}, f.err
	}
	return llm.Response{Content: f.output}, nil
}

func TestRun(t *testing.T) {
	t.Parallel()

	events := []findings.Event{
		{Intro: "Hello."},
		{Finding: &findings.Finding{File: "a.go", Line: 4, Severity: findings.High, Title: "Nil map"}},
		{Finding: &findings.Finding{File: "a.go", Line: 3, Severity: findings.Low, Title: "Style"}},
		{Finding: &findings.Finding{File: "a.go", Line: 1, Severity: findings.Medium, Title: "Unchecked"}},
		{Verdict: "Bye."},
	}
	output := `{"id":1,"verdict":"keep","severity":"critical","evidence":"m[\"x\"] = 1","reason":"m is nil"}` + "\n" +
		`{"id":2,"verdict":"drop"}`

	calls := 0
	res, err := Run(context.Background(), fakeLLM{output: output, calls: &calls}, input(), events, nil)
	require.NoError(t, err)

	assert.Equal(t, 1, calls)
	assert.Equal(t, 2, res.Checked)
	assert.Equal(t, 1, res.Doubted)
	assert.Equal(t, []findings.Event{
		{Intro: "Hello."},
		{Finding: &findings.Finding{File: "a.go", Line: 4, Severity: findings.Critical, Title: "Nil map"}},
		{Finding: &findings.Finding{File: "a.go", Line: 3, Severity: findings.Low, Title: "Style", Doubt: "the check gave no reason"}},
		{Finding: &findings.Finding{File: "a.go", Line: 1, Severity: findings.Medium, Title: "Unchecked"}},
		{Verdict: "Bye."},
	}, res.Events)
	assert.Equal(t, findings.High, events[1].Finding.Severity, "the events passed in are left alone")
}

func TestRun_NoFindingsMakesNoCall(t *testing.T) {
	t.Parallel()

	events := []findings.Event{{Intro: "Hello."}, {Verdict: "Nothing to report."}}
	calls := 0
	res, err := Run(context.Background(), fakeLLM{calls: &calls}, input(), events, func(string, string) *debuglog.Log {
		t.Fatal("no log without a call")
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 0, calls)
	assert.Equal(t, Result{Events: events}, res)
}

func TestRun_Error(t *testing.T) {
	t.Parallel()

	events := []findings.Event{{Finding: &findings.Finding{File: "a.go", Title: "t"}}}
	calls := 0
	_, err := Run(context.Background(), fakeLLM{err: errors.New("boom"), calls: &calls}, input(), events, nil)
	require.ErrorContains(t, err, "boom")
}

// longFile returns a file of n lines, "line 1" to "line n".
func longFile(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

func TestExcerpts(t *testing.T) {
	t.Parallel()

	long := wholeFileLines + 1000
	tests := []struct {
		name  string
		files map[string]string
		fs    []findings.Finding
		want  map[string][][2]int
	}{
		{
			name:  "short file is sent whole",
			files: map[string]string{"a.go": longFile(wholeFileLines)},
			fs:    []findings.Finding{{File: "a.go", Line: 10}},
			want:  map[string][][2]int{"a.go": {{1, wholeFileLines}}},
		},
		{
			name:  "long file is cut to windows around the findings",
			files: map[string]string{"a.go": longFile(long)},
			fs:    []findings.Finding{{File: "a.go", Line: 100}, {File: "a.go", Line: 1000, EndLine: 1010}},
			want: map[string][][2]int{"a.go": {
				{1, 100 + windowLines},
				{1000 - windowLines, 1010 + windowLines},
			}},
		},
		{
			name:  "overlapping windows are merged",
			files: map[string]string{"a.go": longFile(long)},
			fs:    []findings.Finding{{File: "a.go", Line: 1000}, {File: "a.go", Line: 1000 + windowLines}},
			want:  map[string][][2]int{"a.go": {{1000 - windowLines, 1000 + 2*windowLines}}},
		},
		{
			name:  "window at the end of the file",
			files: map[string]string{"a.go": longFile(long)},
			fs:    []findings.Finding{{File: "a.go", Line: long}},
			want:  map[string][][2]int{"a.go": {{long - windowLines, long}}},
		},
		{
			name:  "finding without a line gets the top of the file",
			files: map[string]string{"a.go": longFile(long)},
			fs:    []findings.Finding{{File: "a.go"}},
			want:  map[string][][2]int{"a.go": {{1, 2 * windowLines}}},
		},
		{
			name: "files past the budget are left out",
			files: map[string]string{
				"a.go": longFile(wholeFileLines),
				"b.go": longFile(wholeFileLines),
				"c.go": longFile(wholeFileLines),
				"d.go": longFile(10),
			},
			fs: []findings.Finding{{File: "a.go"}, {File: "b.go"}, {File: "c.go"}, {File: "d.go"}},
			want: map[string][][2]int{
				"a.go": {{1, wholeFileLines}},
				"b.go": {{1, wholeFileLines}},
				"d.go": {{1, 10}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := map[string][][2]int{}
			for _, e := range excerpts(tt.files, tt.fs) {
				got[e.path] = e.ranges
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPromptShowsWindowsOfLongFiles(t *testing.T) {
	t.Parallel()

	long := wholeFileLines + 1000
	in := Input{Diff: diff, Files: map[string]string{"big.go": longFile(long)}}
	got := Prompt(in, []findings.Finding{{File: "big.go", Line: 1000, Title: "t", Body: "b"}})

	assert.Contains(t, got, fmt.Sprintf("### big.go\n\n(%d lines; only the lines around the findings are shown)\n...\n%d\tline %d\n",
		long, 1000-windowLines, 1000-windowLines))
	assert.Contains(t, got, fmt.Sprintf("%d\tline %d\n...\n\n## Findings", 1000+windowLines, 1000+windowLines))
	assert.NotContains(t, got, "\tline 1\n")
}

// Evidence must come from what the check was shown: a quote from a part of
// a long file that wasn't sent doesn't count.
func TestApplyEvidenceOnlyFromSentLines(t *testing.T) {
	t.Parallel()

	in := Input{Diff: diff, Files: map[string]string{"big.go": longFile(wholeFileLines + 1000)}}
	fs := []findings.Finding{{File: "big.go", Line: 1000, Title: "t"}}

	inside := Apply(in, fs, map[int]Verdict{1: {Verdict: "keep", Evidence: "line 1001", Reason: "r"}})
	outside := Apply(in, fs, map[int]Verdict{1: {Verdict: "keep", Evidence: "line 2000", Reason: "r"}})

	assert.True(t, inside[0].Kept)
	assert.False(t, outside[0].Kept)
}
