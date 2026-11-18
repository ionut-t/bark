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
// gh api --paginate --jq prints them: one JSON object per line. They're
// posted by a bot unless untrusted names them.
type fakeReviews struct {
	bodies    []string
	untrusted map[int]bool
	err       error
	args      [][]string
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
		for i, b := range f.bodies {
			r := map[string]string{"body": b, "user_type": "Bot", "author_association": "NONE"}
			if f.untrusted[i] {
				r["user_type"], r["author_association"] = "User", "CONTRIBUTOR"
			}
			line, _ := json.Marshal(r)
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
		`.[] | select(.body | startswith("<!-- bark-review -->")) | {body, user_type: .user.type, author_association} | @json`}, gh.args[1])
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

// A review with no findings still records the commit it reviewed.
func TestBuildRecordsHeadWithoutFindings(t *testing.T) {
	t.Parallel()

	review := build([]findings.Event{{Intro: "i"}, {Verdict: "v"}}, diff, Scope{Head: "abc1234", Base: "fed4321", Since: "0001111"})
	assert.Contains(t, review.Body, recordStart)
	assert.True(t, strings.HasSuffix(review.Body, "-->\n"), review.Body)
	assert.Equal(t, Scope{Head: "abc1234", Base: "fed4321"}, RecordedScope(review.Body))
	assert.Empty(t, Recorded(review.Body), "the head isn't a finding")
}

func TestBuildRecordsHeadWithFindings(t *testing.T) {
	t.Parallel()

	review := build([]findings.Event{
		{Finding: &findings.Finding{File: "a.go", Line: 2, Title: "One", Body: "b"}},
	}, diff, Scope{Head: "abc1234"})
	assert.Equal(t, Scope{Head: "abc1234"}, RecordedScope(review.Body))
	assert.Equal(t, []findings.Finding{{File: "a.go", Line: 2, Title: "One", Body: "b"}}, Recorded(review.Body))
	assert.Equal(t, 1, strings.Count(review.Body, recordStart), "one block")
}

func TestRecordedScope(t *testing.T) {
	t.Parallel()

	assert.Zero(t, RecordedScope(Marker+"\nAn old review without records.\n"))
	assert.Zero(t, RecordedScope(Build([]findings.Event{
		{Finding: &findings.Finding{File: "a.go", Line: 2, Title: "One", Body: "b"}},
	}, diff).Body), "a review from before heads were recorded")
	assert.Zero(t, RecordedScope(recordStart+"\n{\"head\":\"not a sha; rm -rf\"}\n-->\n"), "only a commit hash is taken")
	assert.Equal(t, Scope{Head: "abc1234"}, RecordedScope(recordStart+"\n{\"head\":\"abc1234\",\"base\":\"$(x)\"}\n-->\n"),
		"a base that isn't a hash is dropped")
}

// A review of only the changes since an earlier review says so.
func TestBuildSaysWhatItReviewed(t *testing.T) {
	t.Parallel()

	since := "0123456789abcdef0123456789abcdef01234567"
	body := build([]findings.Event{{Intro: "Hello."}}, diff, Scope{Head: "abc1234", Since: since}).Body
	assert.Contains(t, body, "## Bark AI Code Review\n\n_Reviewed only the changes since the last review, at `0123456`._\n\nHello.")

	body = build([]findings.Event{{Intro: "Hello."}}, diff, Scope{Head: "abc1234"}).Body
	assert.NotContains(t, body, "Reviewed only")
}

// History is what Bark's earlier reviews of a pull request recorded: every
// finding, and the commit the newest of them reviewed.
func TestHistory(t *testing.T) {
	t.Parallel()

	first := build([]findings.Event{
		{Finding: &findings.Finding{File: "a.go", Line: 2, Title: "One", Body: "b"}},
	}, diff, Scope{Head: "aaaaaaa"}).Body
	second := build([]findings.Event{{Intro: "Clean."}}, diff, Scope{Head: "bbbbbbb", Base: "ccccccc"}).Body
	// Posted by a Bark that didn't record heads yet.
	third := Build([]findings.Event{
		{Finding: &findings.Finding{File: "a.go", Line: 3, Title: "Two", Body: "c"}},
	}, diff).Body
	// Posted by someone else, pretending to be Bark.
	forged := build([]findings.Event{
		{Finding: &findings.Finding{File: "a.go", Line: 9, Title: "Forged", Body: "f"}},
	}, diff, Scope{Head: "ddddddd"}).Body
	gh := &fakeReviews{
		bodies:    []string{first, Marker + "\nAn old review without records.\n", second, third, forged},
		untrusted: map[int]bool{4: true},
	}

	got, err := history(context.Background(), gh.run, "7")
	require.NoError(t, err)
	assert.Equal(t, "bbbbbbb", got.Head, "the newest recorded head")
	assert.Equal(t, "ccccccc", got.Base, "with its base")
	assert.Equal(t, []findings.Finding{
		{File: "a.go", Line: 2, Title: "One", Body: "b"},
		{File: "a.go", Line: 3, Title: "Two", Body: "c"},
	}, got.Findings)

	// Who may post Bark's reviews: a bot (GitHub Actions), or someone with
	// write access running Bark locally.
	for _, tc := range []struct {
		userType, association string
		trusted               bool
	}{
		{"Bot", "NONE", true},
		{"User", "OWNER", true},
		{"User", "MEMBER", true},
		{"User", "COLLABORATOR", true},
		{"User", "CONTRIBUTOR", false},
		{"User", "NONE", false},
		{"User", "FIRST_TIME_CONTRIBUTOR", false},
	} {
		assert.Equal(t, tc.trusted, trustedReviewer(tc.userType, tc.association), "%s %s", tc.userType, tc.association)
	}

	none, err := history(context.Background(), (&fakeReviews{}).run, "7")
	require.NoError(t, err)
	assert.Equal(t, History{}, none)
}

// A review that didn't see all its diff (cut to fit the limit, or the
// model's output ended early) records the commit it started from, so the
// next review covers the rest again.
func TestBuildPartialReviewRecordsWhereItStarted(t *testing.T) {
	t.Parallel()

	events := []findings.Event{{Intro: "i"}}
	got := build(events, diff, Scope{Head: "abc1234", Base: "fed4321", Since: "0001111", Partial: true})
	assert.Equal(t, Scope{Head: "0001111", Base: "fed4321"}, RecordedScope(got.Body))

	got = build(events, diff, Scope{Head: "abc1234", Base: "fed4321", Partial: true})
	assert.Zero(t, RecordedScope(got.Body), "a partial review of the whole pull request records no commit")
	assert.NotContains(t, got.Body, recordStart)
}

func TestPostPartialReviewStillGoesOnHead(t *testing.T) {
	t.Parallel()

	gh := &fakeGH{}
	_, err := post(context.Background(), gh.run, "7", []findings.Event{finding(2, 0, "x")}, diff,
		Scope{Head: "def4567", Since: "0001111", Partial: true})
	require.NoError(t, err)
	require.Len(t, gh.posted, 1)
	assert.Equal(t, "def4567", gh.posted[0].CommitID, "the comments are on the lines of the head")
	assert.Equal(t, "0001111", RecordedScope(gh.posted[0].Body).Head)
}
