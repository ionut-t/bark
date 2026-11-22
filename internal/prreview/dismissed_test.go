package prreview

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A finding repeating a dismissed point is set aside in a block of its
// own, gets no inline comment, and isn't recorded: the dismissed point is,
// and its dismissal is read again from the pull request on every review.
func TestBuildPutsDismissedFindingsInCollapsedBlock(t *testing.T) {
	t.Parallel()

	events := []findings.Event{
		{Intro: "Hello."},
		{Finding: &findings.Finding{File: "a.go", Line: 2, Severity: findings.High, Title: "Kept", Body: "b"}},
		{Finding: &findings.Finding{File: "a.go", Line: 3, Severity: findings.Critical, Title: "Again", Body: "c",
			Dismissed: "Same thing (`a.go:3`)"}},
		{Verdict: "Bye."},
	}

	review := Build(events, diff)

	require.Len(t, review.Comments, 1)
	assert.Equal(t, 2, review.Comments[0].Line)
	assert.Contains(t, review.Body, "Hello.\n\n<details>\n<summary>Dismissed on this pull request: 1 finding(s)</summary>\n\n"+
		"Someone with write access dismissed these points on an earlier review (👎 or \"won't fix\"), so they aren't posted again.\n\n"+
		"- **[CRITICAL] Again** `a.go:3`\n  Dismissed point: Same thing (`a.go:3`)\n\n</details>\n\n## Verdict\n\nBye.")
	assert.Equal(t, []findings.Finding{
		{File: "a.go", Line: 2, Title: "Kept", Body: "b"},
	}, Recorded(review.Body))
}

// barkComment is one of Bark's inline comments.
func barkComment(id, review int64, path string, line int, body string, thumbsDown int) github.ReviewComment {
	return github.ReviewComment{ID: id, ReviewID: review, Path: path, Line: line, Body: body,
		User: "github-actions[bot]", UserType: "Bot", Association: "NONE", ThumbsDown: thumbsDown}
}

func reply(id, to int64, user, userType, association, body string) github.ReviewComment {
	return github.ReviewComment{ID: id, ReviewID: 900 + id, InReplyTo: to, Path: "a.go", Line: 1, Body: body,
		User: user, UserType: userType, Association: association}
}

// A 👎 from someone with write access, or a "won't fix" reply from an
// owner, member or collaborator, on one of Bark's inline comments dismisses
// the finding it posted.
func TestHistoryDismissals(t *testing.T) {
	t.Parallel()

	first := Build([]findings.Event{
		{Finding: &findings.Finding{File: "a.go", Line: 2, Title: "Nil map", Body: "m may be nil. It panics."}},
		{Finding: &findings.Finding{File: "a.go", Line: 3, Title: "Ignored error", Body: "e is dropped."}},
		{Finding: &findings.Finding{File: "a.go", Line: 20, Title: "Third", Body: "x."}},
	}, diff).Body
	forged := Build([]findings.Event{
		{Finding: &findings.Finding{File: "a.go", Line: 9, Title: "Forged", Body: "f."}},
	}, diff).Body

	gh := &fakeReviews{
		bodies:    []string{first, forged},
		untrusted: map[int]bool{1: true},
		comments: []github.ReviewComment{
			barkComment(101, 1, "a.go", 2, "**[HIGH · bug] Nil map**\n\nm may be nil. It panics.", 1),
			barkComment(102, 1, "a.go", 3, "**[MEDIUM] Ignored error**\n\ne is dropped.", 0),
			reply(201, 102, "maint", "User", "MEMBER", "Won’t fix, it's on purpose."),
			barkComment(103, 1, "a.go", 20, "**[LOW] Third**\n\nx.", 2),
			reply(202, 103, "drive-by", "User", "CONTRIBUTOR", "won't fix"),
			reply(203, 103, "some-bot[bot]", "Bot", "NONE", "wontfix"),
			// A comment whose finding the review didn't record.
			barkComment(104, 1, "b.go", 7, "**[LOW · style] Not recorded**\n\nFirst. Second.", 1),
			// On the forged review.
			barkComment(105, 2, "a.go", 9, "**[LOW] Forged**\n\nf.", 0),
			reply(204, 105, "boss", "User", "OWNER", "won't fix"),
			// Someone's own review comment.
			barkComment(106, 77, "a.go", 2, "**[LOW] Mine**\n\nm.", 1),
		},
		reactions: map[int64][]github.Reactor{
			101: {{User: "owner", UserType: "User"}},
			103: {{User: "stranger", UserType: "User"}, {User: "dependabot[bot]", UserType: "Bot"}},
			104: {{User: "maint", UserType: "User"}},
		},
		permissions: map[string]string{"owner": "admin", "stranger": "read"},
	}

	got, err := history(context.Background(), gh, "7")
	require.NoError(t, err)
	require.NoError(t, got.DismissalsErr)
	assert.Equal(t, []findings.Finding{
		{File: "a.go", Line: 2, Title: "Nil map", Body: "m may be nil.", Dismissed: "👎 from owner"},
		{File: "a.go", Line: 3, Title: "Ignored error", Body: "e is dropped.", Dismissed: `"won't fix" from maint`},
		{File: "a.go", Line: 20, Title: "Third", Body: "x."},
		{File: "b.go", Line: 7, Title: "Not recorded", Body: "First.", Dismissed: "👎 from maint"},
	}, got.Findings)
	assert.Equal(t, 3, got.Dismissed())

	reactionCalls, permissionCalls := 0, 0
	for _, c := range gh.calls {
		if strings.HasPrefix(c, "ThumbsDown ") {
			reactionCalls++
		}
		if strings.HasPrefix(c, "CanWrite ") {
			permissionCalls++
		}
	}
	assert.Equal(t, 3, reactionCalls, "only Bark comments with a 👎 on a trusted review: %v", gh.calls)
	assert.Equal(t, 2, permissionCalls, "maint is known to be a member; bots aren't asked about: %v", gh.calls)
	assert.Contains(t, gh.calls, "ThumbsDown o/r 101")
	assert.Contains(t, gh.calls, "CanWrite o/r owner")
}

func TestWontFix(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"won't fix", "Won’t fix: intended", "WONT FIX", "wontfix", "I won't fix this now."} {
		assert.True(t, wontFix(s), s)
	}
	for _, s := range []string{"fixed", "will fix", "wont fixate", "won't fixing", ""} {
		assert.False(t, wontFix(s), s)
	}
}

func TestCommentTitle(t *testing.T) {
	t.Parallel()

	title, body, ok := commentFinding("**[HIGH · bug] Nil map**\n\nm may be nil. It panics.\n\n```suggestion\nx\n```")
	require.True(t, ok)
	assert.Equal(t, "Nil map", title)
	assert.Equal(t, "m may be nil.", body)

	title, _, ok = commentFinding("**[LOW] Uses **bold** inside**")
	require.True(t, ok)
	assert.Equal(t, "Uses **bold** inside", title)

	_, _, ok = commentFinding("Just a comment.")
	assert.False(t, ok)
}

// When the comments can't be read, the earlier findings are still used,
// without dismissals, and the error says why.
func TestHistoryDismissalsError(t *testing.T) {
	t.Parallel()

	first := Build([]findings.Event{{Finding: &findings.Finding{File: "a.go", Line: 2, Title: "One", Body: "b"}}}, diff).Body
	gh := &fakeReviews{bodies: []string{first}, commentsErr: errors.New("HTTP 403")}

	got, err := history(context.Background(), gh, "7")
	require.NoError(t, err)
	assert.Equal(t, []findings.Finding{{File: "a.go", Line: 2, Title: "One", Body: "b"}}, got.Findings)
	assert.ErrorContains(t, got.DismissalsErr, "HTTP 403")
}

// A 👎 from someone without write access doesn't count, and isn't an
// error.
func TestHistoryPermissionError(t *testing.T) {
	t.Parallel()

	first := Build([]findings.Event{{Finding: &findings.Finding{File: "a.go", Line: 2, Title: "One", Body: "b"}}}, diff).Body
	gh := &fakeReviews{
		bodies:    []string{first},
		comments:  []github.ReviewComment{barkComment(101, 1, "a.go", 2, "**[HIGH] One**\n\nb", 1)},
		reactions: map[int64][]github.Reactor{101: {{User: "nobody", UserType: "User"}}},
	}
	got, err := history(context.Background(), gh, "7")
	require.NoError(t, err)
	assert.Empty(t, got.Findings[0].Dismissed)
	assert.NoError(t, got.DismissalsErr, "a 404 only means not a collaborator")
}

// An inline comment of a finding Bark's check didn't check starts with a
// note saying so; the finding's body is the paragraph after it.
func TestCommentFindingSkipsUncheckedNote(t *testing.T) {
	t.Parallel()

	title, body, ok := commentFinding("**[HIGH] Nil map**\n\n" + findings.UncheckedNote + " it failed.\n\nm may be nil. It panics.")
	require.True(t, ok)
	assert.Equal(t, "Nil map", title)
	assert.Equal(t, "m may be nil.", body)
}

// A finding posted with the model's own severity tag in its title reads
// back without it, so it matches the same point found again.
func TestPostedTitlesReadBackClean(t *testing.T) {
	t.Parallel()

	title, _, ok := commentFinding("**[MEDIUM · error-handling] [major] Non-atomic reply**\n\nThe reply comes first.")
	require.True(t, ok)
	assert.Equal(t, "Non-atomic reply", title)

	got := Recorded("<!-- bark-findings v1\n" + `{"file":"a.go","line":1,"title":"[minor] Missing ellipsis","body":"b"}` + "\n-->")
	require.Len(t, got, 1)
	assert.Equal(t, "Missing ellipsis", got[0].Title)
}
