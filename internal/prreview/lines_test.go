package prreview

import (
	"errors"
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeLines maps lines of a.go at commit "c1" as if two lines were
// inserted above line 10 and line 30 was changed since; other files and
// commits fail.
func fakeLines(commit, path string, start, end int) (int, int, bool, error) {
	if commit != "c1" || path != "a.go" {
		return 0, 0, false, errors.New("no such commit")
	}
	if start <= 30 && end >= 30 {
		return 0, 0, false, nil
	}
	if start >= 10 {
		return start + 2, end + 2, true, nil
	}
	return start, end, true, nil
}

// A finding on the lines of a dismissed point, where those lines haven't
// changed since its review, is matched to that point, as a hint for the
// check; it decides whether it's the same problem.
func TestDismissedOnLines(t *testing.T) {
	t.Parallel()

	prior := []findings.Finding{
		{File: "a.go", Line: 12, EndLine: 14, Title: "Nil map", Commit: "c1", Dismissed: "👎 from o"},
		{File: "a.go", Line: 29, EndLine: 31, Title: "Changed since", Commit: "c1", Dismissed: "👎 from o"},
		{File: "a.go", Line: 40, Title: "Not dismissed", Commit: "c1"},
		{File: "a.go", Line: 50, Title: "No commit", Dismissed: "👎 from o"},
	}
	tests := []struct {
		name    string
		finding findings.Finding
		want    string
	}{
		{"overlaps the moved lines", findings.Finding{File: "a.go", Line: 16, Title: "Other title"}, "Nil map"},
		{"a range overlapping them", findings.Finding{File: "a.go", Line: 10, EndLine: 14, Title: "t"}, "Nil map"},
		{"the old line numbers don't count", findings.Finding{File: "a.go", Line: 12, Title: "t"}, ""},
		{"lines changed since", findings.Finding{File: "a.go", Line: 31, Title: "t"}, ""},
		{"a point not dismissed", findings.Finding{File: "a.go", Line: 42, Title: "t"}, ""},
		{"a point without its commit", findings.Finding{File: "a.go", Line: 50, Title: "t"}, ""},
		{"another file", findings.Finding{File: "b.go", Line: 16, Title: "t"}, ""},
		{"no line", findings.Finding{File: "a.go", Title: "t"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fs := []findings.Finding{{File: "z.go", Line: 1, Title: "unrelated"}, tt.finding}
			got, err := DismissedOnLines(fs, prior, fakeLines)
			require.NoError(t, err)
			if tt.want == "" {
				assert.Empty(t, got)
				return
			}
			require.Contains(t, got, 1, "keyed by the finding's index")
			assert.Equal(t, tt.want, got[1].Title)
			assert.Len(t, got, 1)
		})
	}
}

// A point dismissed under one title is dismissed under the others it was
// recorded with in the same file, so their (later) lines can match too.
func TestDismissedOnLinesFollowsTitles(t *testing.T) {
	t.Parallel()

	prior := []findings.Finding{
		{File: "a.go", Line: 2, Title: "Nil map", Commit: "c0", Dismissed: "👎 from o"},
		{File: "a.go", Line: 12, Title: "nil map", Commit: "c1"},
	}
	got, err := DismissedOnLines([]findings.Finding{{File: "a.go", Line: 14, Title: "t"}}, prior, fakeLines)
	require.Contains(t, got, 0)
	assert.Equal(t, 12, got[0].Line)
	assert.ErrorContains(t, err, "no such commit", "the c0 lookup failed and is reported")
}

// The points get the commit their review was posted on, which their line
// numbers refer to.
func TestHistoryRecordsCommits(t *testing.T) {
	t.Parallel()

	first := Build([]findings.Event{{Finding: &findings.Finding{File: "a.go", Line: 2, Title: "One", Body: "b"}}}, diff).Body
	gh := &fakeReviews{bodies: []string{first}, commits: map[int]string{0: "abc1234"}}
	got, err := history(t.Context(), gh.run, "7")
	require.NoError(t, err)
	assert.Equal(t, []findings.Finding{{File: "a.go", Line: 2, Title: "One", Body: "b", Commit: "abc1234"}}, got.Findings)
}

// A dismissed comment whose finding its review didn't record gets the
// review's commit too, so its lines can be matched.
func TestHistoryRecordsCommitOfUnrecordedDismissal(t *testing.T) {
	t.Parallel()

	gh := &fakeReviews{
		bodies:   []string{Marker + "\nAn old review without records.\n"},
		commits:  map[int]string{0: "abc1234"},
		comments: []map[string]any{barkComment(101, 1, "a.go", 7, "**[LOW] Not recorded**\n\nb.", 0), reply(201, 101, "o", "User", "OWNER", "won't fix")},
	}
	got, err := history(t.Context(), gh.run, "7")
	require.NoError(t, err)
	assert.Equal(t, []findings.Finding{{File: "a.go", Line: 7, Title: "Not recorded", Body: "b.",
		Dismissed: `"won't fix" from o`, Commit: "abc1234"}}, got.Findings)
}
