package prreview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const postedOn = "c0ffee1234567890c0ffee1234567890c0ffee12"

// postedComment is one of Bark's inline comments on a.go as the review
// comments query prints it, covering lines line..end of commit postedOn.
func postedComment(id, line, end int, body string) map[string]any {
	c := barkComment(id, 1, "a.go", line, body, 0)
	c["end_line"], c["commit"] = end, postedOn
	return c
}

func thread(id string, comment int, resolved bool) map[string]any {
	return map[string]any{"id": id, "resolved": resolved, "comment": comment}
}

// The threads Bark may resolve once its check finds them fixed are its own
// open ones that nobody answered and nobody dismissed. A thread resolved on
// the pull request makes its point resolved, so an incremental approval
// doesn't wait for it.
func TestHistoryThreads(t *testing.T) {
	t.Parallel()

	body := Build([]findings.Event{
		{Finding: &findings.Finding{File: "a.go", Line: 2, Title: "Nil map", Body: "m may be nil. It panics."}},
		{Finding: &findings.Finding{File: "a.go", Line: 3, Title: "Answered", Body: "x."}},
		{Finding: &findings.Finding{File: "a.go", Line: 4, Title: "Dismissed", Body: "x."}},
		{Finding: &findings.Finding{File: "a.go", Line: 5, Title: "Resolved", Body: "x."}},
		{Finding: &findings.Finding{File: "a.go", Line: 6, Title: "Unresolved again", Body: "x."}},
		{Finding: &findings.Finding{File: "a.go", Line: 8, Title: "Bot replied", Body: "y."}},
		{Finding: &findings.Finding{File: "a.go", Line: 9, Title: "Fixed earlier", Body: "z."}},
	}, diff).Body

	gh := &fakeReviews{
		bodies: []string{body},
		comments: []map[string]any{
			postedComment(101, 2, 3, "**[HIGH · bug] Nil map**\n\nm may be nil. It panics.\n\n```suggestion\nx\n```"),
			postedComment(102, 3, 3, "**[LOW] Answered**\n\nx."),
			reply(201, 102, "maint", "User", "MEMBER", "Not sure, let's talk."),
			postedComment(103, 4, 4, "**[LOW] Dismissed**\n\nx."),
			reply(202, 103, "maint", "User", "MEMBER", "won't fix"),
			postedComment(104, 5, 5, "**[LOW] Resolved**\n\nx."),
			// Bark found it fixed, and someone unresolved the thread.
			postedComment(105, 6, 6, "**[LOW] Unresolved again**\n\nx."),
			reply(203, 105, "maint", "User", "OWNER", fixedMarker+"\nFixed in `abc1234`: the won't fix comment is gone."),
			postedComment(106, 8, 8, "**[LOW] Bot replied**\n\n"+findings.UncheckedNote+" it failed.\n\ny."),
			reply(204, 106, "some-bot[bot]", "Bot", "NONE", "Thanks!"),
			// Bark found it fixed and resolved it.
			postedComment(107, 9, 9, "**[LOW] Fixed earlier**\n\nz."),
			reply(205, 107, "github-actions[bot]", "Bot", "NONE", fixedMarker+"\nFixed in `abc1234`: gone."),
		},
		threads: []map[string]any{
			thread("T101", 101, false), thread("T102", 102, false), thread("T103", 103, false),
			thread("T104", 104, true), thread("T105", 105, false), thread("T106", 106, false),
			thread("T107", 107, true),
		},
	}

	got, err := history(context.Background(), gh.run, "7")
	require.NoError(t, err)
	require.NoError(t, got.ThreadsErr)

	pull := "repos/o/r/pulls/7"
	assert.Equal(t, []Thread{
		{ID: "T101", Comment: 101, pull: pull, Point: findings.Finding{
			File: "a.go", Line: 2, EndLine: 3, Title: "Nil map", Body: "m may be nil. It panics.", Commit: postedOn}},
		{ID: "T106", Comment: 106, pull: pull, Point: findings.Finding{
			File: "a.go", Line: 8, EndLine: 8, Title: "Bot replied", Body: "y.", Commit: postedOn}},
	}, got.Threads)

	resolved, fixed := map[string]string{}, map[string]bool{}
	for _, f := range got.Findings {
		resolved[f.Title] = f.Resolved
		fixed[f.Title] = f.Fixed
	}
	assert.Equal(t, map[string]string{
		"Nil map": "", "Answered": "", "Dismissed": "", "Resolved": "resolved on the pull request",
		"Unresolved again": "", "Bot replied": "", "Fixed earlier": "fixed, as Bark's check found",
	}, resolved)
	assert.Equal(t, map[string]bool{
		"Nil map": false, "Answered": false, "Dismissed": false, "Resolved": false,
		"Unresolved again": false, "Bot replied": false, "Fixed earlier": true,
	}, fixed, "only a thread Bark resolved as fixed is a fixed point")
	assert.Equal(t, `"won't fix" from maint`, got.Findings[2].Dismissed)
	assert.Empty(t, got.Findings[4].Dismissed, "Bark's reply saying it's fixed isn't a dismissal, whatever it quotes")

	i := slices.IndexFunc(gh.args, func(a []string) bool { return a[1] == "graphql" })
	require.GreaterOrEqual(t, i, 0)
	assert.Equal(t, []string{"api", "graphql", "--paginate", "-f", "query=" + threadsQuery,
		"-f", "owner=o", "-f", "repo=r", "-F", "number=7", "--jq", threadsJQ}, gh.args[i])
}

// Without an inline comment of Bark's, the threads aren't read; when they
// can't be, the history is still good and says why.
func TestHistoryThreadsError(t *testing.T) {
	t.Parallel()

	body := Build([]findings.Event{{Finding: &findings.Finding{File: "a.go", Line: 2, Title: "One", Body: "b"}}}, diff).Body

	gh := &fakeReviews{bodies: []string{body}}
	got, err := history(context.Background(), gh.run, "7")
	require.NoError(t, err)
	assert.Empty(t, got.Threads)
	assert.False(t, slices.ContainsFunc(gh.args, func(a []string) bool { return a[1] == "graphql" }))

	gh = &fakeReviews{
		bodies:     []string{body},
		comments:   []map[string]any{postedComment(101, 2, 2, "**[LOW] One**\n\nb")},
		threadsErr: errors.New("HTTP 502"),
	}
	got, err = history(context.Background(), gh.run, "7")
	require.NoError(t, err)
	assert.Len(t, got.Findings, 1)
	assert.Empty(t, got.Threads)
	assert.ErrorContains(t, got.ThreadsErr, "HTTP 502")
}

// A comment Bark posted on a commit it can't read back into git, or not on
// a line, can't be compared with the code now.
func TestHistoryThreadsNeedCommitAndLine(t *testing.T) {
	t.Parallel()

	body := Build([]findings.Event{{Finding: &findings.Finding{File: "a.go", Line: 2, Title: "One", Body: "b"}}}, diff).Body
	noCommit := postedComment(101, 2, 2, "**[LOW] One**\n\nb")
	noCommit["commit"] = "main; rm -rf /"
	noLine := postedComment(102, 0, 0, "**[LOW] One**\n\nb")
	gh := &fakeReviews{
		bodies:   []string{body},
		comments: []map[string]any{noCommit, noLine},
		threads:  []map[string]any{thread("T101", 101, false), thread("T102", 102, false)},
	}
	got, err := history(context.Background(), gh.run, "7")
	require.NoError(t, err)
	assert.Empty(t, got.Threads)
}

// fakeResolve answers replies and the resolveReviewThread and
// unresolveReviewThread mutations, failing for the comments and threads
// given.
type fakeResolve struct {
	replyErr     map[int64]error
	resolveErr   map[string]error
	unresolveErr map[string]error
	replies      map[int64]string
	resolved     map[string]bool
	order        []string
}

func (f *fakeResolve) run(_ context.Context, stdin []byte, args ...string) ([]byte, error) {
	if args[0] == "api" && args[1] == "graphql" {
		id := strings.TrimPrefix(args[len(args)-1], "id=")
		if f.resolved == nil {
			f.resolved = map[string]bool{}
		}
		switch {
		case slices.Contains(args, "query="+resolveMutation):
			f.order = append(f.order, "resolve "+id)
			if err := f.resolveErr[id]; err != nil {
				return nil, err
			}
			f.resolved[id] = true
		case slices.Contains(args, "query="+unresolveMutation):
			f.order = append(f.order, "unresolve "+id)
			if err := f.unresolveErr[id]; err != nil {
				return nil, err
			}
			delete(f.resolved, id)
		default:
			return nil, errors.New("unexpected query")
		}
		return []byte(`{"data":{}}`), nil
	}
	var id int64
	if args[0] == "api" && len(args) == 6 && args[1] == "--method" && args[2] == "POST" {
		if _, err := fmt.Sscanf(args[3], "repos/o/r/pulls/7/comments/%d/replies", &id); err == nil {
			f.order = append(f.order, fmt.Sprintf("reply %d", id))
			if err := f.replyErr[id]; err != nil {
				return nil, err
			}
			var body struct {
				Body string `json:"body"`
			}
			if err := json.Unmarshal(stdin, &body); err != nil {
				return nil, err
			}
			if f.replies == nil {
				f.replies = map[int64]string{}
			}
			f.replies[id] = body.Body
			return []byte(`{}`), nil
		}
	}
	return nil, errors.New("unexpected gh call " + strings.Join(args, " "))
}

func openThread(id string, comment int64, title string) Thread {
	return Thread{ID: id, Comment: comment, pull: "repos/o/r/pulls/7",
		Point: findings.Finding{File: "a.go", Line: 2, Title: title, Commit: postedOn}}
}

// Bark resolves each fixed thread, then says on it why. A thread it
// couldn't resolve gets no reply, so a missing permission posts nothing;
// one it couldn't reply on is unresolved again, so a resolved thread always
// says why.
func TestResolve(t *testing.T) {
	t.Parallel()

	gh := &fakeResolve{
		resolveErr:   map[string]error{"T102": errors.New("HTTP 403")},
		replyErr:     map[int64]error{103: errors.New("HTTP 502"), 104: errors.New("HTTP 503")},
		unresolveErr: map[string]error{"T104": errors.New("HTTP 404")},
	}
	fixed := []Fixed{
		{Thread: openThread("T101", 101, "Nil map"), Reason: "m is checked\n  for nil   now."},
		{Thread: openThread("T102", 102, "Two"), Reason: "x"},
		{Thread: openThread("T103", 103, "Three"), Reason: "y"},
		{Thread: openThread("T104", 104, "Four"), Reason: "z"},
	}
	got, err := resolve(context.Background(), gh.run, fixed, "abcdef1234567890")

	assert.Equal(t, []Thread{fixed[0].Thread}, got)
	require.Error(t, err)
	assert.ErrorContains(t, err, `failed to resolve the thread of "Two": HTTP 403`)
	assert.ErrorContains(t, err, `failed to reply on the thread of "Three", so it was unresolved again: HTTP 502`)
	assert.ErrorContains(t, err, `failed to reply on the thread of "Four": HTTP 503`)
	assert.ErrorContains(t, err, `failed to unresolve it again, so it's resolved without saying why: HTTP 404`)
	assert.Equal(t, []string{"resolve T101", "reply 101", "resolve T102", "resolve T103", "reply 103", "unresolve T103",
		"resolve T104", "reply 104", "unresolve T104"}, gh.order)
	assert.Equal(t, map[string]bool{"T101": true, "T104": true}, gh.resolved)
	assert.Equal(t, []int64{101}, slices.Sorted(maps.Keys(gh.replies)))
	assert.Equal(t, fixedMarker+"\nFixed in `abcdef1`: m is checked for nil now.\n\n"+
		"_Bark's check found this fixed, so it resolved the thread. Unresolve it if it isn't fixed._", gh.replies[101])
}

// A point whose thread Bark resolved no longer counts as open.
func TestHistoryMarkResolved(t *testing.T) {
	t.Parallel()

	h := History{Findings: []findings.Finding{
		{File: "a.go", Title: "Nil map"},
		{File: "b.go", Title: "Nil map"},
		{File: "a.go", Title: "Other"},
		{File: "a.go", Title: "nil  MAP", Resolved: "resolved on the pull request"},
	}}
	h.MarkResolved([]Thread{openThread("T101", 101, "Nil map")}, "abcdef1234")
	assert.Equal(t, []string{"fixed in abcdef1", "", "", "resolved on the pull request"}, []string{
		h.Findings[0].Resolved, h.Findings[1].Resolved, h.Findings[2].Resolved, h.Findings[3].Resolved,
	})
	assert.True(t, h.Findings[0].Fixed)
	assert.False(t, h.Findings[3].Fixed)
}
