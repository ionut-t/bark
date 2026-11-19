package check

import (
	"context"
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dismissedPrior is prior with its first point dismissed on the pull
// request.
func dismissedPrior() []findings.Finding {
	p := prior()
	p[0].Dismissed = "👎 from octocat"
	return p
}

func TestPromptMarksDismissedPoints(t *testing.T) {
	t.Parallel()

	in := input()
	in.Prior = dismissedPrior()
	got := Prompt(in, []findings.Finding{{File: "a.go", Line: 4, Severity: findings.High, Title: "t", Body: "b"}})

	assert.Contains(t, got, "1. `a.go:3` Nil map write: m may be nil. (dismissed on this pull request: won't fix)\n"+
		"2. `b.go:10-12` Ignored error: The error is dropped.\n")
}

// A point is dismissed if any copy of it was, even an older one.
func TestPriorPointsKeepDismissal(t *testing.T) {
	t.Parallel()

	got := priorPoints([]findings.Finding{
		{File: "a.go", Line: 3, Title: "Nil map write", Dismissed: "👎 from octocat"},
		{File: "a.go", Line: 5, Title: "nil map write"},
	})
	require.Len(t, got, 1)
	assert.Equal(t, 5, got[0].Line, "the most recent copy is kept")
	assert.Equal(t, "👎 from octocat", got[0].Dismissed)
}

func TestApplyDismissed(t *testing.T) {
	t.Parallel()

	in := input()
	in.Prior = dismissedPrior()
	f := findings.Finding{File: "a.go", Line: 4, Severity: findings.High, Title: "t"}

	got := Apply(in, []findings.Finding{f}, map[int]Verdict{1: {Verdict: "drop", Reason: "Raised before (1): same."}})
	assert.Equal(t, []Outcome{{Finding: f, Reason: "Raised before (1): same.", Dismissed: "Nil map write (`a.go:3`)"}}, got)
}

func TestRunMarksDismissed(t *testing.T) {
	t.Parallel()

	in := input()
	in.Prior = dismissedPrior()
	events := []findings.Event{
		{Finding: &findings.Finding{File: "a.go", Line: 4, Severity: findings.High, Title: "Nil map"}},
		{Verdict: "Bye."},
	}
	calls := 0
	res, err := Run(context.Background(), fakeLLM{output: `{"id":1,"verdict":"drop","reason":"Raised before (1): same."}`, calls: &calls},
		in, events, nil)
	require.NoError(t, err)

	assert.Equal(t, 1, res.Checked)
	assert.Equal(t, 1, res.Dismissed)
	assert.Zero(t, res.RaisedBefore)
	assert.Zero(t, res.Doubted)
	assert.Equal(t, "Nil map write (`a.go:3`)", res.Events[0].Finding.Dismissed)
	assert.Empty(t, res.Events[0].Finding.RaisedBefore)
}

// A rewritten verdict mustn't ask for a dismissed point to be fixed.
func TestRevisePromptSetsDismissedAside(t *testing.T) {
	t.Parallel()

	got := RevisePrompt([]findings.Event{
		{Finding: &findings.Finding{File: "b.go", Severity: findings.Low, Title: "Again", Dismissed: "Earlier"}},
		{Verdict: "v"},
	})
	assert.NotContains(t, got, "Still open")
	assert.Contains(t, got, "## Findings set aside\n\n- [LOW] Again (b.go)\n")
}

// The same problem is often listed under several titles: the check is
// asked to name the dismissed one, so its repeat is dismissed, not only
// raised before.
func TestSystemPrefersDismissedPoint(t *testing.T) {
	t.Parallel()

	assert.Contains(t, System(), "give the number of one marked as dismissed")
}

// A finding on the unchanged lines of a dismissed point is pointed out to
// the check, which decides whether it's the same problem.
func TestPromptNotesSameLines(t *testing.T) {
	t.Parallel()

	in := input()
	in.Prior = dismissedPrior()
	in.SameLines = map[int]findings.Finding{1: in.Prior[0]}
	fs := []findings.Finding{
		{File: "b.go", Line: 11, Severity: findings.Low, Title: "u", Body: "b"},
		{File: "a.go", Line: 4, Severity: findings.High, Title: "t", Body: "b"},
	}

	got := Prompt(in, fs)
	assert.Contains(t, got, "\n2. "+findingJSON(fs[1])+"\n"+
		"   Note: on the same lines as point 1 (dismissed), which haven't changed since.\n")
	assert.NotContains(t, got, "\n1. "+findingJSON(fs[0])+"\n   Note")

	in.SameLines = map[int]findings.Finding{1: {File: "a.go", Line: 99, Title: "Not listed"}}
	assert.NotContains(t, Prompt(in, fs), "Note:", "no note for a point the check isn't shown")
}

func TestSystemExplainsSameLines(t *testing.T) {
	t.Parallel()

	assert.Contains(t, System(), "on the same lines as a dismissed point")
}
