package plain

import (
	"errors"
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/check"
	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/prreview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Dismissed findings aren't kept: they don't count for fail_on.
func TestCheckSummaryWithDismissed(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "bark: the check kept 2 of 4 finding(s); 1 raised before on this pull request, not posted again; "+
		"1 dismissed on this pull request, not posted\n",
		checkSummary(check.Result{Checked: 4, Doubted: 1, RaisedBefore: 1, Dismissed: 1}))
}

func TestGateSkipsDismissed(t *testing.T) {
	t.Parallel()

	var w strings.Builder
	require.NoError(t, gate(&w, []findings.Event{
		{Finding: &findings.Finding{File: "a.go", Line: 1, Severity: findings.Critical, Title: "Won't fix", Dismissed: "Earlier"}},
	}, findings.Low))
	assert.Empty(t, w.String())
}

func TestHistoryNoteWithDismissals(t *testing.T) {
	t.Parallel()

	h := prreview.History{Findings: []findings.Finding{{Title: "a", Dismissed: "👎 from o"}, {Title: "b"}}}
	var w strings.Builder
	historyNote(&w, h, nil, true, true)
	assert.Equal(t, "bark: earlier reviews of this pull request raised 2 finding(s); 1 dismissed\n", w.String())

	h.DismissalsErr = errors.New("failed to read review comments: HTTP 403")
	w.Reset()
	historyNote(&w, h, nil, true, true)
	assert.Equal(t, "bark: earlier reviews of this pull request raised 2 finding(s); 1 dismissed\n"+
		"bark: warning: failed to read review comments: HTTP 403; some dismissals (👎 or \"won't fix\") may be missed\n", w.String())

	w.Reset()
	historyNote(&w, h, nil, false, true)
	assert.Empty(t, w.String(), "dismissals only matter to the check")
}

// The hint for the check: which findings are on unchanged lines of a
// dismissed point. Failed lookups are a warning.
func TestSameLinesHint(t *testing.T) {
	t.Parallel()

	prior := []findings.Finding{
		{File: "a.go", Line: 10, Title: "Nil map", Commit: "c1", Dismissed: "👎 from o"},
		{File: "b.go", Line: 10, Title: "Other", Commit: "c9", Dismissed: "👎 from o"},
	}
	mapLines := func(commit, _ string, start, end int) (int, int, bool, error) {
		if commit != "c1" {
			return 0, 0, false, errors.New("commit isn't in the local repository: c9")
		}
		return start + 1, end + 1, true, nil
	}
	fs := []findings.Finding{
		{File: "a.go", Line: 11, Severity: findings.Critical, Title: "Again"},
		{File: "b.go", Line: 11, Severity: findings.High, Title: "Kept"},
	}

	var w strings.Builder
	got := sameLinesHint(&w, prior, mapLines)(fs)
	assert.Equal(t, map[int]findings.Finding{0: prior[0]}, got)
	assert.Equal(t, "bark: warning: couldn't match some dismissed points by line: commit isn't in the local repository: c9\n", w.String())

	assert.Nil(t, sameLinesHint(&w, prior, nil), "no hint without a pull request head")
}
