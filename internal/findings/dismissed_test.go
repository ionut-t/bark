package findings

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A finding repeating a point someone dismissed on the pull request (a 👎
// or a "won't fix" reply) is set aside and doesn't block a merge.
func TestDismissedFindings(t *testing.T) {
	t.Parallel()

	dismissed := Finding{File: "d.go", Line: 4, Severity: Critical, Title: "Won't fix", Dismissed: "Nil map (`d.go:3`)"}
	assert.True(t, dismissed.SetAside())

	events := []Event{
		{Finding: &Finding{File: "a.go", Line: 1, Severity: Critical, Title: "Kept"}},
		{Finding: &dismissed},
	}
	assert.Len(t, Blocking(events, Low), 1, "a dismissed finding doesn't count for fail_on")
}

func TestMarkdownMovesDismissedFindingsBeforeVerdict(t *testing.T) {
	t.Parallel()

	events := []Event{
		{Intro: "i"},
		{Finding: &Finding{File: "a.go", Line: 1, Severity: High, Title: "Kept"}},
		{Finding: &Finding{File: "c.go", Line: 89, Severity: Medium, Title: "Ignored error", Dismissed: "Ignored error (`c.go:88`)"}},
		{Verdict: "v"},
	}

	assert.Equal(t, "i\n\n"+
		"### [HIGH] Kept\n\n`a.go:1`\n\n"+
		"## Dismissed on this pull request\n\n"+
		"Someone with write access dismissed these points on an earlier review (👎 or \"won't fix\"), so they aren't repeated as findings.\n\n"+
		"- **[MEDIUM] Ignored error** `c.go:89`\n  Dismissed point: Ignored error (`c.go:88`)\n\n"+
		"## Verdict\n\nv\n\n", Markdown(events))

	assert.Equal(t, "### [MEDIUM] Ignored error\n\n"+
		"> **Dismissed on this pull request:** Ignored error (`c.go:88`)\n\n"+
		"`c.go:89`\n\n", MarkedMarkdown(events[2:3]))
}

func TestCheckReportWithDismissed(t *testing.T) {
	t.Parallel()

	kept := Event{Finding: &Finding{File: "a.go", Line: 1, Severity: High, Title: "Kept"}}
	dismissed := Event{Finding: &Finding{File: "c.go", Line: 3, Severity: Low, Title: "Again", Dismissed: "Earlier (`c.go:3`)"}}

	assert.Equal(t, "## Bark's check\n\nA second pass checked 2 findings against the code. "+
		"1 was dismissed on this pull request:\n\n"+
		"- **[LOW] Again** `c.go:3`\n  Dismissed point: Earlier (`c.go:3`)\n\n",
		CheckReport([]Event{kept, dismissed}, 2))
}
