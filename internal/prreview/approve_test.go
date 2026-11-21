package prreview

import (
	"context"
	"errors"
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostApproves(t *testing.T) {
	t.Parallel()

	gh := &fakeGH{}
	got, err := post(context.Background(), gh.run, "7", []findings.Event{finding(2, 0, "")}, diff, Scope{}, true)
	require.NoError(t, err)
	assert.True(t, got.Approved)
	assert.NoError(t, got.ApprovalErr)
	require.Len(t, gh.posted, 1)
	assert.Equal(t, "APPROVE", gh.posted[0].Event)
	assert.Len(t, gh.posted[0].Comments, 1)
}

func TestPostWithoutApproveComments(t *testing.T) {
	t.Parallel()

	gh := &fakeGH{}
	got, err := post(context.Background(), gh.run, "7", []findings.Event{finding(2, 0, "")}, diff, Scope{}, false)
	require.NoError(t, err)
	assert.False(t, got.Approved)
	require.Len(t, gh.posted, 1)
	assert.Equal(t, "COMMENT", gh.posted[0].Event)
}

// GitHub refuses approvals from GitHub Actions unless the repository allows
// them, and from a pull request's author. The review is still posted, with
// its inline comments, as a comment.
func TestPostFallsBackToCommentWhenApprovalRefused(t *testing.T) {
	t.Parallel()

	refused := errors.New("HTTP 422: GitHub Actions is not permitted to approve pull requests.")
	gh := &fakeGH{postErrs: []error{refused}}
	got, err := post(context.Background(), gh.run, "7", []findings.Event{finding(2, 0, "")}, diff, Scope{}, true)
	require.NoError(t, err)
	assert.False(t, got.Approved)
	assert.ErrorContains(t, got.ApprovalErr, "not permitted to approve")
	assert.False(t, got.Fallback)
	assert.Equal(t, 1, got.Inline)
	require.Len(t, gh.posted, 2)
	assert.Equal(t, "COMMENT", gh.posted[1].Event)
	assert.Len(t, gh.posted[1].Comments, 1)
}

// When the inline comments are what GitHub rejects, the approval still goes
// through with every finding in the body.
func TestPostApprovesWithBodyFallback(t *testing.T) {
	t.Parallel()

	lines := errors.New("HTTP 422: Line could not be resolved")
	gh := &fakeGH{postErrs: []error{lines, lines}}
	got, err := post(context.Background(), gh.run, "7", []findings.Event{finding(2, 0, "")}, diff, Scope{}, true)
	require.NoError(t, err)
	assert.True(t, got.Approved)
	assert.NoError(t, got.ApprovalErr)
	assert.True(t, got.Fallback)
	require.Len(t, gh.posted, 3)
	assert.Equal(t, "APPROVE", gh.posted[2].Event)
	assert.Empty(t, gh.posted[2].Comments)
}

func TestPostReportsEveryErrorWhenApprovalAndCommentsFail(t *testing.T) {
	t.Parallel()

	gh := &fakeGH{postErrs: []error{errors.New("one"), errors.New("two"), errors.New("three"), errors.New("four")}}
	_, err := post(context.Background(), gh.run, "7", []findings.Event{finding(2, 0, "")}, diff, Scope{}, true)
	require.Error(t, err)
	for _, want := range []string{"one", "two", "three", "four"} {
		assert.ErrorContains(t, err, want)
	}
	assert.Len(t, gh.posted, 4)
}

// When the inline comments and the approval are both rejected, the reason
// given is the one for the approval posted without inline comments.
func TestPostReportsApprovalErrorWithoutInlineComments(t *testing.T) {
	t.Parallel()

	lines := errors.New("HTTP 422: Line could not be resolved")
	refused := errors.New("HTTP 422: Can not approve your own pull request")
	gh := &fakeGH{postErrs: []error{lines, lines, refused}}
	got, err := post(context.Background(), gh.run, "7", []findings.Event{finding(2, 0, "")}, diff, Scope{}, true)
	require.NoError(t, err)
	assert.False(t, got.Approved)
	assert.True(t, got.Fallback)
	assert.ErrorContains(t, got.ApprovalErr, "your own pull request")
	require.Len(t, gh.posted, 4)
	assert.Equal(t, "COMMENT", gh.posted[3].Event)
}
