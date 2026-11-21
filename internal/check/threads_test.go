package check

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	postedOn = "c0ffee1234567890c0ffee1234567890c0ffee12"
	// beforeA is a.go when an earlier review raised a point on line 4;
	// fixedA is a.go now, with the point fixed.
	beforeA = "package a\n\nfunc f(m map[string]int) {\n\tm[\"x\"] = 1\n}\n"
	fixedA  = "package a\n\nfunc f(m map[string]int) {\n\tif m == nil {\n\t\treturn\n\t}\n\tm[\"x\"] = 1\n}\n"
)

func nilMapThread() Thread {
	return Thread{
		Point: findings.Finding{File: "a.go", Line: 4, EndLine: 4, Title: "Nil map", Body: "m may be nil. It panics.", Commit: postedOn},
		Line:  4, EndLine: 7,
		Before: beforeA,
	}
}

func threadInput(threads ...Thread) Input {
	return Input{Diff: diff, Files: map[string]string{"a.go": fixedA}, Threads: threads}
}

func TestPromptShowsOpenThreads(t *testing.T) {
	t.Parallel()

	gone := Thread{
		Point:  findings.Finding{File: "a.go", Line: 1, Title: "Package name", Body: "b", Commit: postedOn},
		Before: beforeA,
	}
	got := Prompt(threadInput(nilMapThread(), gone), nil)

	assert.NotContains(t, got, "## Findings", "no findings, no section")
	assert.Contains(t, got, "### a.go\n\n1\tpackage a\n", "the thread's file is shown")
	assert.Contains(t, got, "## Open threads\n\n")
	assert.Contains(t, got, "1. `a.go:4` Nil map: m may be nil. It panics.\n"+
		"   Then, at `c0ffee1`:\n"+
		"   ```\n"+
		"   4\t\tm[\"x\"] = 1\n"+
		"   ```\n"+
		"   Now: lines 4-7 of the new a.go.\n")
	assert.Contains(t, got, "2. `a.go:1` Package name: b\n"+
		"   Then, at `c0ffee1`:\n"+
		"   ```\n"+
		"   1\tpackage a\n"+
		"   ```\n"+
		"   Now: nothing is left of those lines.\n")
}

// A thread is fixed only when the check quotes code of the new file that
// wasn't there when the point was raised: positive evidence of a change
// that fixes it, never the code that was already there.
func TestApplyThreads(t *testing.T) {
	t.Parallel()

	threads := []Thread{
		nilMapThread(), nilMapThread(), nilMapThread(), nilMapThread(), nilMapThread(), nilMapThread(),
		nilMapThread(), nilMapThread(), nilMapThread(),
	}
	noBefore := nilMapThread()
	noBefore.Before = ""
	threads = append(threads, noBefore, nilMapThread())
	verdicts := parseThreadVerdicts(strings.Join([]string{
		`{"thread":1,"verdict":"fixed","evidence":"if m == nil {\n\t\treturn","reason":"f returns early on a nil map."}`,
		`{"thread":2,"verdict":"fixed","evidence":"m[\"x\"] = 1","reason":"Looks fine."}`,
		`{"thread":3,"verdict":"open","evidence":"if m == nil {","reason":"Still panics."}`,
		`{"thread":4,"verdict":"fixed","evidence":"if m != nil && ok {","reason":"Made up."}`,
		`{"thread":5,"verdict":"fixed","evidence":"if m == nil {"}`,
		`{"id":6,"verdict":"keep","evidence":"if m == nil {","reason":"A finding's verdict."}`,
		// An elided line isn't new code, whichever ellipsis marks it.
		`{"thread":7,"verdict":"fixed","evidence":"m[\"x\"] = 1\n…","reason":"Elided."}`,
		`{"thread":8,"verdict":"fixed","evidence":"m[\"x\"] = 1\n...","reason":"Elided."}`,
		// Thread 9 gets no verdict.
		`{"thread":10,"verdict":"fixed","evidence":"if m == nil {","reason":"Checked now."}`,
		`{"thread":3,"verdict":"open","evidence":"","reason":"  Still panics:\n m is written first.  "}`,
		// No line is new, but the write now comes after a block it didn't.
		`{"thread":11,"verdict":"fixed","evidence":"}\n\tm[\"x\"] = 1","reason":"The nil check comes first."}`,
	}, "\n"))

	fixed, open := applyThreads(threadInput(threads...), nil, verdicts)
	assert.Equal(t, map[int]string{0: "f returns early on a nil map.", 10: "The nil check comes first."}, fixed)
	const old = "the check called it fixed, but the code it quoted was already there when the point was raised"
	assert.Equal(t, map[int]string{
		1: old,
		2: "Still panics:\n m is written first.",
		3: "the check called it fixed, but the code it quoted isn't in the code now",
		4: "the check called it fixed without a reason",
		5: "the check gave no verdict", // a finding's verdict isn't a thread's
		6: old,
		7: old,
		8: "the check gave no verdict",
		9: "the check called it fixed, but Bark couldn't read the code it was about then",
	}, open)
}

func TestRunWithOnlyThreads(t *testing.T) {
	t.Parallel()

	events := []findings.Event{{Intro: "Hi."}, {Verdict: "Fine."}}
	calls := 0
	res, err := Run(context.Background(), fakeLLM{
		output: `{"thread":1,"verdict":"fixed","evidence":"if m == nil {","reason":"Checked now."}`, calls: &calls,
	}, threadInput(nilMapThread()), events, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, calls, "open threads are checked without findings")
	assert.Equal(t, events, res.Events)
	assert.Equal(t, map[int]string{0: "Checked now."}, res.Fixed)
	assert.Empty(t, res.Open)

	_, err = Run(context.Background(), fakeLLM{output: "Can't tell.", calls: &calls}, threadInput(nilMapThread()), events, nil)
	require.EqualError(t, err, "the check gave no verdict on any of the 1 open thread(s)")
}

// Threads and findings are checked in the same call.
func TestRunWithFindingsAndThreads(t *testing.T) {
	t.Parallel()

	events := []findings.Event{{Finding: &findings.Finding{File: "a.go", Line: 5, Severity: findings.High, Title: "Other"}}}
	calls := 0
	res, err := Run(context.Background(), fakeLLM{
		output: `{"id":1,"verdict":"drop","reason":"No."}` + "\n" +
			`{"thread":1,"verdict":"fixed","evidence":"if m == nil {","reason":"Checked now."}`, calls: &calls,
	}, threadInput(nilMapThread()), events, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Equal(t, 1, res.Doubted)
	assert.Equal(t, map[int]string{0: "Checked now."}, res.Fixed)
}

func TestSystemExplainsOpenThreads(t *testing.T) {
	t.Parallel()

	assert.Contains(t, System(), `{"thread":1,"verdict":"fixed","evidence":"...","reason":"..."}`)
}

// When the check drops a finding as a repeat of a point and also calls
// that point's thread fixed, it contradicts itself: the thread stays open.
func TestRunKeepsThreadOpenWhenItsPointIsRaisedAgain(t *testing.T) {
	t.Parallel()

	in := threadInput(nilMapThread())
	in.Prior = []findings.Finding{{File: "a.go", Line: 4, Title: "nil  map", Body: "m may be nil.", Commit: postedOn}}
	events := []findings.Event{{Finding: &findings.Finding{File: "a.go", Line: 7, Severity: findings.High, Title: "Map may be nil"}}}
	calls := 0
	res, err := Run(context.Background(), fakeLLM{
		output: `{"id":1,"verdict":"drop","reason":"Raised before (1): the same nil map."}` + "\n" +
			`{"thread":1,"verdict":"fixed","evidence":"if m == nil {","reason":"Checked now."}`, calls: &calls,
	}, in, events, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, res.Events[0].Finding.RaisedBefore)
	assert.Empty(t, res.Fixed)
	assert.Equal(t, map[int]string{0: "the check called it fixed, but also set aside a new finding as a repeat of its point"}, res.Open)
}

func TestLinesOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		file       string
		start, end int
		want       string
	}{
		{"lines", "a\nb\nc\n", 2, 3, "2\tb\n3\tc\n"},
		{"past the end", "a\n", 1, 3, "1\ta\n"},
		{"empty file", "", 1, 1, ""},
		{"no start", "a\n", 0, 1, ""},
		{"start past the end", "a\n", 2, 2, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, linesOf(tt.file, tt.start, tt.end))
		})
	}
}

// A fix that reorders or removes code adds no line, so the lines quoted
// as evidence count when they're together in the new file and weren't in
// the old one. Lines quoted from all over, or a block that was already
// there, don't.
func TestNewCode(t *testing.T) {
	t.Parallel()

	before := "func f() {\n\ta()\n\tb()\n\tlog(x)\n\tc()\n}\n"
	after := "func f() {\n\tb()\n\n\ta()\n\tc()\n}\n"
	tests := []struct {
		name     string
		evidence string
		want     bool
	}{
		{"a new line", "d()", true},
		{"now next to the start", "func f() {\n\tb()", true},
		{"reordered", "b()\na()", true},
		{"reordered, across a blank line", "b()\n\na()", true},
		{"removed between", "a()\nc()", true},
		{"with line numbers", "4\ta()\n5\tc()", true},
		{"one line", "a()", false},
		{"already together", "c()\n}", false},
		{"not together now", "b()\nc()", false},
		{"elided between", "b()\n...\nc()", false},
		{"elided, one part new", "func f() {\n…\na()\nc()", true},
		{"only short lines", "}\n}", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, newCode(tt.evidence, before, after))
		})
	}
}

// The fix that made PR 155's "Non-atomic" thread fixed resolved the
// thread before replying: no line was new, only their order.
func TestNewCodeReorderedCalls(t *testing.T) {
	t.Parallel()

	before, err := os.ReadFile("testdata/resolve_before.go.txt")
	require.NoError(t, err)
	after, err := os.ReadFile("testdata/resolve_after.go.txt")
	require.NoError(t, err)

	evidence := `if _, err := run(ctx, nil, "api", "graphql", "-f", "query="+resolveMutation, "-f", "id="+t.ID); err != nil {
	errs = append(errs, fmt.Errorf("failed to resolve the thread of %q: %w", t.Point.Title, err))
	continue
}
if _, err := run(ctx, payload, "api", "--method", "POST",`
	assert.False(t, newLine(evidence, string(before)), "every line was there before")
	assert.True(t, newCode(evidence, string(before), string(after)))

	unchanged := `if err != nil {
	errs = append(errs, err)
	continue
}`
	assert.False(t, newCode(unchanged, string(before), string(after)))
}
