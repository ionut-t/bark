package plain

import (
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var secretDiff = "diff --git a/aws.go b/aws.go\n" +
	"--- a/aws.go\n" +
	"+++ b/aws.go\n" +
	"@@ -1,1 +1,2 @@\n" +
	" package aws\n" +
	"+const key = \"" + "AKIA" + "IOSFODNN7EXAMPLE" + "\"\n"

const secretTitle = "Hard-coded secret (aws-access-key-id)"

func TestSecretFindings(t *testing.T) {
	t.Parallel()

	events := secretFindings(secretDiff, nil)
	require.Len(t, events, 1)
	f := events[0].Finding
	require.NotNil(t, f)
	assert.Equal(t, "aws.go", f.File)
	assert.Equal(t, 2, f.Line)
	assert.Equal(t, findings.Critical, f.Severity)
	assert.Equal(t, "security", f.Category)
	assert.Equal(t, secretTitle, f.Title)
	assert.Contains(t, f.Body, "rotate")
	assert.NotContains(t, f.Body, "IOSFODNN7EXAMPLE")
	assert.Empty(t, f.Suggestion)

	assert.Nil(t, secretFindings("+++ b/a.go\n@@ -1 +1 @@\n-x := 1\n+x := 2\n", nil))
}

// On a pull request, a secret Bark reported before isn't posted again but
// still counts; one dismissed there (a 👎 on a test fixture, say) is set
// aside.
func TestSecretFindingsRaisedBefore(t *testing.T) {
	t.Parallel()

	raised := secretFindings(secretDiff, []findings.Finding{{File: "aws.go", Line: 3, Title: secretTitle}})
	require.Len(t, raised, 1)
	assert.Equal(t, secretTitle+" (`aws.go:3`)", raised[0].Finding.RaisedBefore)
	assert.Empty(t, raised[0].Finding.Dismissed)
	assert.Len(t, findings.Blocking(raised, findings.Critical), 1)

	dismissed := secretFindings(secretDiff, []findings.Finding{{File: "aws.go", Line: 2, Title: strings.ToLower(secretTitle), Dismissed: "👎"}})
	require.Len(t, dismissed, 1)
	assert.Equal(t, strings.ToLower(secretTitle)+" (`aws.go:2`)", dismissed[0].Finding.Dismissed)
	assert.Empty(t, findings.Blocking(dismissed, findings.Low))

	other := secretFindings(secretDiff, []findings.Finding{{File: "other.go", Line: 2, Title: secretTitle}})
	assert.Empty(t, other[0].Finding.RaisedBefore)
}

// Secret findings come first in the review, and aren't sent to the check.
func TestFinishReviewWithSecrets(t *testing.T) {
	t.Parallel()

	secrets := secretFindings(secretDiff, nil)
	var checked []findings.Event
	checkFn := func(events []findings.Event) ([]findings.Event, int) {
		checked = events
		return events, 0
	}

	var out strings.Builder
	events := finishReview(&out, structuredOutput, false, false, true, "", checkFn, secrets)

	for _, e := range checked {
		if e.Finding != nil {
			assert.NotEqual(t, secretTitle, e.Finding.Title, "the check was sent a secret finding")
		}
	}
	require.NotEmpty(t, events)
	require.NotNil(t, events[0].Finding)
	assert.Equal(t, secretTitle, events[0].Finding.Title)
	assert.True(t, strings.HasPrefix(out.String(), "### [CRITICAL · security] "+secretTitle), out.String())
	assert.Len(t, findings.Blocking(events, findings.Critical), 1)

	// Without the check, they're still added.
	var plainOut strings.Builder
	events = finishReview(&plainOut, structuredOutput, false, false, true, "", nil, secrets)
	assert.Equal(t, secretTitle, events[0].Finding.Title)
}

// A streamed review has shown the secret findings already; the check's
// summary after it is about what the check did, so a secret raised before
// on the pull request isn't listed there as if the check had found it.
func TestFinishReviewStreamedCheckReportLeavesOutSecrets(t *testing.T) {
	t.Parallel()

	secrets := secretFindings(secretDiff, []findings.Finding{{File: "aws.go", Line: 2, Title: secretTitle}})
	require.NotEmpty(t, secrets[0].Finding.RaisedBefore)
	keep := func(events []findings.Event) ([]findings.Event, int) { return events, 1 }

	var out strings.Builder
	events := finishReview(&out, structuredOutput, false, true, true, "", keep, secrets)

	assert.Equal(t, "## Bark's check\n\nA second pass checked 1 finding against the code and kept it.\n\n", out.String())
	assert.Equal(t, secretTitle, events[0].Finding.Title, "the secret is still a finding")
}
