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

func TestBuildPutsRaisedBeforeFindingsInCollapsedBlock(t *testing.T) {
	t.Parallel()

	events := []findings.Event{
		{Intro: "Hello."},
		{Finding: &findings.Finding{File: "a.go", Line: 2, Severity: findings.High, Title: "Kept", Body: "b"}},
		{Finding: &findings.Finding{File: "a.go", Line: 3, Severity: findings.Low, Category: "design", Title: "Again",
			Body: "b", RaisedBefore: "Same thing (`a.go:3`)"}},
		{Verdict: "Bye."},
	}

	review := Build(events, diff)

	require.Len(t, review.Comments, 1)
	assert.Equal(t, 2, review.Comments[0].Line)
	assert.Contains(t, review.Body, "Hello.\n\n<details>\n<summary>Raised before on this pull request: 1 finding(s)</summary>\n\n"+
		"An earlier review of this pull request already raised these, so they aren't posted again.\n\n"+
		"- **[LOW · design] Again** `a.go:3`\n  Same point as: Same thing (`a.go:3`)\n\n</details>\n\n## Verdict\n\nBye.")
}

func TestBuildRecordsFindings(t *testing.T) {
	t.Parallel()

	events := []findings.Event{
		{Intro: "Hello."},
		{Finding: &findings.Finding{File: "a.go", Line: 2, Severity: findings.High, Title: "Kept",
			Body: "First sentence. Second sentence.", Suggestion: "x", Remark: "Ugh."}},
		{Finding: &findings.Finding{File: "a.go", Line: 3, EndLine: 4, Title: "Ends a comment -->", Body: "b", Doubt: "No."}},
		{Finding: &findings.Finding{File: "a.go", Line: 20, Title: "Again", Body: "c", RaisedBefore: "Earlier"}},
		{Verdict: "Bye."},
	}

	for _, d := range []string{diff, ""} {
		review := Build(events, d)

		assert.True(t, strings.HasSuffix(review.Body, "-->\n"), review.Body)
		hidden := review.Body[strings.LastIndex(review.Body, recordStart):]
		assert.Equal(t, 1, strings.Count(hidden, "-->"), "a title can't end the hidden block")
		assert.Equal(t, []findings.Finding{
			{File: "a.go", Line: 2, Title: "Kept", Body: "First sentence."},
			{File: "a.go", Line: 3, EndLine: 4, Title: "Ends a comment -->", Body: "b"},
			{File: "a.go", Line: 20, Title: "Again", Body: "c"},
		}, Recorded(review.Body))
	}
}

func TestBuildWithoutFindingsRecordsNothing(t *testing.T) {
	t.Parallel()

	review := Build([]findings.Event{{Intro: "i"}, {Verdict: "v"}}, diff)
	assert.NotContains(t, review.Body, recordStart)
	assert.Empty(t, Recorded(review.Body))
}

func TestRecorded(t *testing.T) {
	t.Parallel()

	assert.Empty(t, Recorded(Marker+"\n## Bark AI Code Review\n\nA review from before findings were recorded.\n"))
	assert.Equal(t, []findings.Finding{{File: "a.go", Line: 1, Title: "t"}},
		Recorded(recordStart+"\n{\"file\":\"a.go\",\"line\":1,\"title\":\"t\"}\nnot json\n{\"title\":\"no file\"}\n-->\n"),
		"lines that aren't records are skipped")

	// A visible title quoting the block's start doesn't hide the real block.
	body := Build([]findings.Event{
		{Finding: &findings.Finding{File: "a.go", Line: 2, Title: recordStart, Body: "b"}},
	}, diff).Body
	assert.Equal(t, []findings.Finding{{File: "a.go", Line: 2, Title: recordStart, Body: "b"}}, Recorded(body))
}

// fakeReviews answers gh pr view and lists the reviews in bodies, as
// gh api --paginate --jq prints them: one JSON string per line.
type fakeReviews struct {
	bodies []string
	err    error
	args   [][]string
}

func (f *fakeReviews) run(_ context.Context, _ []byte, args ...string) ([]byte, error) {
	f.args = append(f.args, args)
	switch args[0] {
	case "pr":
		return []byte(`{"url":"https://github.com/o/r/pull/7","headRefOid":"abc123"}`), nil
	case "api":
		if f.err != nil {
			return nil, f.err
		}
		var out []byte
		for _, b := range f.bodies {
			line, _ := json.Marshal(b)
			out = append(append(out, line...), '\n')
		}
		return out, nil
	}
	return nil, errors.New("unexpected gh call")
}

func TestEarlier(t *testing.T) {
	t.Parallel()

	first := Build([]findings.Event{
		{Finding: &findings.Finding{File: "a.go", Line: 2, Title: "One", Body: "b"}},
	}, diff).Body
	second := Build([]findings.Event{
		{Finding: &findings.Finding{File: "a.go", Line: 3, Title: "Two", Body: "c", Doubt: "No."}},
	}, diff).Body
	gh := &fakeReviews{bodies: []string{first, Marker + "\nAn old review without records.\n", second}}

	got, err := earlier(context.Background(), gh.run, "7")
	require.NoError(t, err)
	assert.Equal(t, []findings.Finding{
		{File: "a.go", Line: 2, Title: "One", Body: "b"},
		{File: "a.go", Line: 3, Title: "Two", Body: "c"},
	}, got)

	require.Len(t, gh.args, 2)
	assert.Equal(t, []string{"api", "--paginate", "repos/o/r/pulls/7/reviews", "--jq",
		`.[] | select(.body | startswith("<!-- bark-review -->")) | .body | @json`}, gh.args[1])
}

func TestEarlierError(t *testing.T) {
	t.Parallel()

	gh := &fakeReviews{err: errors.New("HTTP 404")}
	_, err := earlier(context.Background(), gh.run, "7")
	assert.ErrorContains(t, err, "HTTP 404")
}

// The hidden block keeps a short body, so it stays small however long the
// findings are.
func TestRecordCutsLongBodies(t *testing.T) {
	t.Parallel()

	body := Build([]findings.Event{
		{Finding: &findings.Finding{File: "a.go", Line: 2, Title: "t", Body: strings.Repeat("word ", 500)}},
	}, diff).Body
	got := Recorded(body)
	require.Len(t, got, 1)
	assert.Equal(t, findings.FirstSentence(strings.Repeat("word ", 500)), got[0].Body)
	assert.Less(t, len(got[0].Body), 300)
}
