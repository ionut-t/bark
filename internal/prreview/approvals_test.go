package prreview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Bark's approvals still in force are read with its earlier reviews; a
// dismissed one, a comment, and an untrusted look-alike aren't.
func TestHistoryApprovals(t *testing.T) {
	t.Parallel()

	body := Build([]findings.Event{{Verdict: "Fine."}}, diff).Body
	gh := &fakeReviews{
		bodies:    []string{body, body, body, body},
		commits:   map[int]string{0: "aaaaaaa1", 1: "bbbbbbb2", 3: "ddddddd4"},
		states:    map[int]string{0: "APPROVED", 1: "DISMISSED", 3: "APPROVED"},
		untrusted: map[int]bool{3: true},
	}
	got, err := history(context.Background(), gh.run, "7")
	require.NoError(t, err)
	assert.Equal(t, []Approval{{ID: 1, Commit: "aaaaaaa1", pull: "repos/o/r/pulls/7"}}, got.Approvals)
}

// fakeDismiss answers review dismissals, failing for the reviews given.
type fakeDismiss struct {
	err       map[int64]error
	dismissed map[int64]string
}

func (f *fakeDismiss) run(_ context.Context, stdin []byte, args ...string) ([]byte, error) {
	var id int64
	if len(args) == 6 && args[0] == "api" && args[1] == "--method" && args[2] == "PUT" && args[4] == "--input" {
		if _, err := fmt.Sscanf(args[3], "repos/o/r/pulls/7/reviews/%d/dismissals", &id); err == nil {
			if err := f.err[id]; err != nil {
				return nil, err
			}
			var body struct {
				Message string `json:"message"`
				Event   string `json:"event"`
			}
			if err := json.Unmarshal(stdin, &body); err != nil || body.Event != "DISMISS" {
				return nil, fmt.Errorf("bad dismissal %s", stdin)
			}
			if f.dismissed == nil {
				f.dismissed = map[int64]string{}
			}
			f.dismissed[id] = body.Message
			return []byte(`{}`), nil
		}
	}
	return nil, fmt.Errorf("unexpected gh call %v", args)
}

// A review that doesn't approve withdraws Bark's earlier approvals, saying
// why; the ones it couldn't dismiss are reported.
func TestWithdraw(t *testing.T) {
	t.Parallel()

	gh := &fakeDismiss{err: map[int64]error{2: errors.New("HTTP 403")}}
	approvals := []Approval{
		{ID: 1, Commit: "aaaaaaa1111", pull: "repos/o/r/pulls/7"},
		{ID: 2, Commit: "bbbbbbb2222", pull: "repos/o/r/pulls/7"},
	}
	n, err := withdraw(context.Background(), gh.run, approvals, "cccccccc3333", "the review kept 1 finding(s) at or above medium severity")

	assert.Equal(t, 1, n)
	assert.NotContains(t, err.Error(), "aaaaaaa", "the dismissed approval isn't an error")
	assert.ErrorContains(t, err, "failed to dismiss Bark's approval of bbbbbbb: HTTP 403")
	assert.Equal(t, map[int64]string{
		1: "Bark's review of ccccccc doesn't approve this pull request: the review kept 1 finding(s) at or above medium severity.",
	}, gh.dismissed)
}
