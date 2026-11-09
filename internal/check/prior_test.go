package check

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// prior is what earlier reviews of a pull request raised.
func prior() []findings.Finding {
	return []findings.Finding{
		{File: "a.go", Line: 3, Title: "Nil map write", Body: "m may be nil. It panics."},
		{File: "b.go", Line: 10, EndLine: 12, Title: "Ignored error", Body: "The error is dropped."},
	}
}

func TestPromptListsPointsRaisedBefore(t *testing.T) {
	t.Parallel()

	in := input()
	in.Prior = prior()
	fs := []findings.Finding{{File: "a.go", Line: 4, Severity: findings.High, Title: "t", Body: "b"}}

	got := Prompt(in, fs)

	assert.Contains(t, got, "\n## Raised before on this pull request\n\n"+
		"Earlier reviews of this pull request already reported these points. Line numbers are from earlier commits.\n\n"+
		"1. `a.go:3` Nil map write: m may be nil.\n"+
		"2. `b.go:10-12` Ignored error: The error is dropped.\n"+
		"\n## Findings\n\n")
	assert.NotContains(t, Prompt(input(), fs), "Raised before", "no section without earlier points")
}

// The same point raised in several earlier reviews is listed once, and
// only the most recent points are listed when there are many.
func TestPriorPoints(t *testing.T) {
	t.Parallel()

	dup := []findings.Finding{
		{File: "a.go", Line: 3, Title: "Nil map write"},
		{File: "a.go", Line: 5, Title: "nil map write"},
		{File: "b.go", Line: 3, Title: "Nil map write"},
	}
	got := priorPoints(dup)
	require.Len(t, got, 2)
	assert.Equal(t, 5, got[0].Line, "the most recent copy is kept")
	assert.Equal(t, "b.go", got[1].File)

	var many []findings.Finding
	for i := range maxPriorPoints + 5 {
		many = append(many, findings.Finding{File: "a.go", Line: i + 1, Title: fmt.Sprint("t", i)})
	}
	got = priorPoints(many)
	require.Len(t, got, maxPriorPoints)
	assert.Equal(t, 6, got[0].Line)
}

func TestApplyRaisedBefore(t *testing.T) {
	t.Parallel()

	in := input()
	in.Prior = prior()
	finding := findings.Finding{File: "a.go", Line: 4, Severity: findings.High, Title: "t"}

	tests := []struct {
		name    string
		finding findings.Finding
		verdict Verdict
		want    Outcome
	}{
		{
			name:    "repeat of a point in the same file",
			finding: finding,
			verdict: Verdict{Verdict: "drop", Reason: "Raised before (1): the same nil map write."},
			want: Outcome{Finding: finding, Reason: "Raised before (1): the same nil map write.",
				RaisedBefore: "Nil map write (`a.go:3`)"},
		},
		{
			name:    "prefix in another case and spacing",
			finding: finding,
			verdict: Verdict{Verdict: "drop", Reason: "  raised before(1) - same point"},
			want:    Outcome{Finding: finding, Reason: "  raised before(1) - same point", RaisedBefore: "Nil map write (`a.go:3`)"},
		},
		{
			name:    "a point in another file isn't the same point",
			finding: finding,
			verdict: Verdict{Verdict: "drop", Reason: "Raised before (2): ignored error."},
			want:    Outcome{Finding: finding, Reason: "Raised before (2): ignored error."},
		},
		{
			name:    "no such point",
			finding: finding,
			verdict: Verdict{Verdict: "drop", Reason: "Raised before (7)."},
			want:    Outcome{Finding: finding, Reason: "Raised before (7)."},
		},
		{
			name:    "a kept finding isn't a repeat",
			finding: finding,
			verdict: Verdict{Verdict: "keep", Evidence: "package a", Reason: "Raised before (1), but still wrong."},
			want:    Outcome{Finding: finding, Kept: true, Reason: "Raised before (1), but still wrong."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Apply(in, []findings.Finding{tt.finding}, map[int]Verdict{1: tt.verdict})
			assert.Equal(t, []Outcome{tt.want}, got)
		})
	}
}

func TestRunMarksRaisedBefore(t *testing.T) {
	t.Parallel()

	in := input()
	in.Prior = prior()
	events := []findings.Event{
		{Finding: &findings.Finding{File: "a.go", Line: 4, Severity: findings.High, Title: "Nil map"}},
		{Finding: &findings.Finding{File: "a.go", Line: 3, Severity: findings.Low, Title: "Style"}},
		{Verdict: "Bye."},
	}
	output := `{"id":1,"verdict":"drop","reason":"Raised before (1): same nil map."}` + "\n" +
		`{"id":2,"verdict":"drop","reason":"A preference."}`

	calls := 0
	res, err := Run(context.Background(), fakeLLM{output: output, calls: &calls}, in, events, nil)
	require.NoError(t, err)

	assert.Equal(t, 2, res.Checked)
	assert.Equal(t, 1, res.Doubted)
	assert.Equal(t, 1, res.RaisedBefore)
	assert.Equal(t, []findings.Event{
		{Finding: &findings.Finding{File: "a.go", Line: 4, Severity: findings.High, Title: "Nil map",
			RaisedBefore: "Nil map write (`a.go:3`)"}},
		{Finding: &findings.Finding{File: "a.go", Line: 3, Severity: findings.Low, Title: "Style", Doubt: "A preference."}},
		{Verdict: "Bye."},
	}, res.Events)
}

// Apply reads the reason prefix the system prompt asks for.
func TestSystemAsksForRaisedBeforePrefix(t *testing.T) {
	t.Parallel()

	assert.Contains(t, System(), `starts with "Raised before (N):"`)
	assert.True(t, strings.Contains(System(), "the points earlier reviews of this pull request raised"))
}

func TestNeedsRevisionWithRaisedBefore(t *testing.T) {
	t.Parallel()

	repeat := findings.Event{Finding: &findings.Finding{File: "b.go", Severity: findings.Low, Title: "Again", RaisedBefore: "Earlier"}}

	assert.True(t, NeedsRevision([]findings.Event{repeat, {Verdict: "Fix it again."}}))
	assert.Contains(t, RevisePrompt([]findings.Event{repeat, {Verdict: "v"}}), "## Findings set aside\n\n- [LOW] Again (b.go)\n")
}
