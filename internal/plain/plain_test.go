package plain

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/check"
	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/ionut-t/bark/v2/internal/prreview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeLLM returns output whole from Generate, and from Stream in chunks of
// chunkSize bytes, so lines are split across chunks.
type fakeLLM struct {
	output    string
	chunkSize int
	truncated bool
}

func (f fakeLLM) Stream(context.Context, string, string) (<-chan llm.Response, <-chan error) {
	out := make(chan llm.Response)
	errc := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errc)
		for i := 0; i < len(f.output); i += f.chunkSize {
			out <- llm.Response{Content: f.output[i:min(i+f.chunkSize, len(f.output))]}
		}
		if f.truncated {
			out <- llm.Response{Truncated: true}
		}
	}()
	return out, errc
}

func (f fakeLLM) Generate(context.Context, string, string) (llm.Response, error) {
	return llm.Response{Content: f.output, Truncated: f.truncated}, nil
}

const structuredOutput = `{"file":"a.go","line":2,"severity":"high","category":"bug","title":"Nil map","body":"m is nil.","remark":"Illogical."}` + "\n" +
	`{"type":"verdict","text":"Fix the map."}`

const renderedReview = "### [HIGH · bug] Nil map\n\n`a.go:2`\n\nm is nil.\n\n> _Illogical._\n\n" +
	"## Verdict\n\nFix the map.\n\n\n"

func TestReviewOutput(t *testing.T) {
	t.Parallel()

	client := fakeLLM{output: structuredOutput, chunkSize: 7}

	tests := []struct {
		name       string
		stream     bool
		structured bool
		want       string
	}{
		{name: "structured, streamed", stream: true, structured: true, want: renderedReview},
		{name: "structured, whole", stream: false, structured: true, want: renderedReview},
		{name: "free text, streamed", stream: true, structured: false, want: structuredOutput + "\n"},
		{name: "free text, whole", stream: false, structured: false, want: structuredOutput + "\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out strings.Builder
			var raw string
			var err error
			if tt.stream {
				raw, _, err = streamResponse(context.Background(), client, "", "", &out, tt.structured, "", nil)
			} else {
				raw, _, err = fullResponse(context.Background(), client, "", "", nil)
			}
			require.NoError(t, err)
			finishReview(&out, raw, false, tt.stream, tt.structured, "", nil)
			assert.Equal(t, tt.want, out.String())
			assert.Equal(t, structuredOutput, raw)
		})
	}
}

// A review the model cut off at its output limit says so at the end, and a
// structured one carries the note as its last event, for the posted review.
func TestReviewOutputTruncated(t *testing.T) {
	t.Parallel()

	client := fakeLLM{output: structuredOutput, chunkSize: 7, truncated: true}

	tests := []struct {
		name       string
		stream     bool
		structured bool
		want       string
	}{
		{name: "structured, streamed", stream: true, structured: true, want: renderedReview + findings.TruncatedNote},
		{name: "structured, whole", stream: false, structured: true, want: strings.TrimSuffix(renderedReview, "\n") + findings.TruncatedNote + "\n\n"},
		{name: "free text, streamed", stream: true, structured: false, want: structuredOutput + "\n\n" + findings.TruncatedNote},
		{name: "free text, whole", stream: false, structured: false, want: structuredOutput + "\n\n" + findings.TruncatedNote},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out strings.Builder
			var raw string
			var truncated bool
			var err error
			if tt.stream {
				raw, truncated, err = streamResponse(context.Background(), client, "", "", &out, tt.structured, "", nil)
			} else {
				raw, truncated, err = fullResponse(context.Background(), client, "", "", nil)
			}
			require.NoError(t, err)
			require.True(t, truncated)
			events := finishReview(&out, raw, truncated, tt.stream, tt.structured, "", nil)
			assert.Equal(t, tt.want, out.String())
			if tt.structured {
				require.NotEmpty(t, events)
				assert.Equal(t, findings.TruncatedNote, events[len(events)-1].Text)
			}
		})
	}
}

// Findings below min_severity are never shown or checked, streamed or not.
func TestReviewOutputMinSeverity(t *testing.T) {
	t.Parallel()

	client := fakeLLM{output: structuredOutput, chunkSize: 7}
	const want = "## Verdict\n\nFix the map.\n\n\n"

	for _, stream := range []bool{true, false} {
		var out strings.Builder
		var raw string
		var err error
		if stream {
			raw, _, err = streamResponse(context.Background(), client, "", "", &out, true, findings.Critical, nil)
		} else {
			raw, _, err = fullResponse(context.Background(), client, "", "", nil)
		}
		require.NoError(t, err)

		events := finishReview(&out, raw, false, stream, true, findings.Critical, func(events []findings.Event) ([]findings.Event, int) {
			assert.Zero(t, check.Count(events), "findings below the threshold mustn't be checked")
			return events, 0
		})
		assert.Equal(t, want, out.String(), "stream %v", stream)
		assert.Len(t, events, 1, "only the verdict is left")
	}
}

// With the check, a whole review shows the doubted findings in their own
// section; a streamed one, already on screen, gets the check's report
// appended.
func TestFinishReviewWithCheck(t *testing.T) {
	t.Parallel()

	doubtAll := func(events []findings.Event) ([]findings.Event, int) {
		out := make([]findings.Event, len(events))
		for i, e := range events {
			if e.Finding != nil {
				f := *e.Finding
				f.Doubt = "m is never nil."
				e.Finding = &f
			}
			out[i] = e
		}
		return out, 1
	}
	doubted := "- **[HIGH · bug] Nil map** `a.go:2`\n  m is never nil.\n\n"

	t.Run("whole", func(t *testing.T) {
		t.Parallel()

		var out strings.Builder
		events := finishReview(&out, structuredOutput, false, false, true, "", doubtAll)

		assert.Equal(t, "## Doubted by Bark's check\n\n"+
			"A second pass checked these findings against the code and doubts them. They are listed in case it's wrong.\n\n"+
			doubted+"## Verdict\n\nFix the map.\n\n\n", out.String())
		require.Len(t, events, 2)
		assert.Equal(t, "m is never nil.", events[0].Finding.Doubt)
	})

	t.Run("streamed", func(t *testing.T) {
		t.Parallel()

		var out strings.Builder
		finishReview(&out, structuredOutput, false, true, true, "", doubtAll)

		assert.Equal(t, "## Bark's check\n\nA second pass checked 1 finding against the code and doubts 1:\n\n"+doubted, out.String())
	})

	t.Run("free text is never checked", func(t *testing.T) {
		t.Parallel()

		var out strings.Builder
		events := finishReview(&out, "plain text", false, false, false, "", func([]findings.Event) ([]findings.Event, int) {
			t.Fatal("free text has no findings to check")
			return nil, 0
		})
		assert.Nil(t, events)
		assert.Equal(t, "plain text\n", out.String())
	})
}

// A review of a diff cut to fit its limit starts with a note saying so,
// also when it's posted to the pull request.
func TestPartialReviewNote(t *testing.T) {
	t.Parallel()

	events := findings.Parse(structuredOutput)
	assert.Empty(t, partialReviewNote(nil))
	assert.Equal(t, events, withPartialNote(events, nil))

	cut := &git.DiffCut{Files: 5, Kept: 2, Setting: "max_diff_tokens", Dropped: []string{"c_test.go", "d.md", "e.md"}}
	note := partialReviewNote(cut)
	assert.Equal(t, "> **Partial review:** Bark reviewed only 2 of 5 changed files: the diff was cut to fit max_diff_tokens, code first and tests, docs, fixtures and generated files last. Left out: `c_test.go`, `d.md` and `e.md`.\n", note)

	got := withPartialNote(events, cut)
	require.Len(t, got, len(events)+1)
	assert.Equal(t, note, got[0].Text)
	assert.Equal(t, events, got[1:])

	body := prreview.Build(got, "").Body
	assert.Contains(t, body, "## Bark AI Code Review\n\n"+note, "the note comes first in the posted review")
}

// Warnings about the diff go to stderr, so the review on stdout stays
// clean for pipes.
func TestWarnReviewDiff(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder
	warnReviewDiff(&stderr, git.ReviewDiff{Warnings: []string{"could not read pull request #7's title", "second"}})
	assert.Equal(t, "bark: warning: could not read pull request #7's title\nbark: warning: second\n", stderr.String())

	stderr.Reset()
	warnReviewDiff(&stderr, git.ReviewDiff{})
	assert.Empty(t, stderr.String())
}

// A pull request review says on stderr whether it reviews the whole pull
// request or only what changed since the last review, and why.
func TestNoteReviewScope(t *testing.T) {
	t.Parallel()

	reviewed := "0123456789abcdef0123456789abcdef01234567"
	tests := []struct {
		name     string
		full     bool
		recorded string
		r        git.ReviewDiff
		want     string
	}{
		{"incremental", false, reviewed,
			git.ReviewDiff{Since: reviewed, Diff: "d", Commits: []git.Commit{{}, {}}},
			"bark: reviewing only the changes since the last review at 0123456 (2 commit(s))\n"},
		{"no record", false, "", git.ReviewDiff{Diff: "d"},
			"bark: full review: no earlier Bark review of this pull request recorded the commit it reviewed\n"},
		{"fallback", false, reviewed, git.ReviewDiff{Diff: "d", FullReason: "0123456 is no longer in the pull request's history (rebase or force-push)"},
			"bark: full review: 0123456 is no longer in the pull request's history (rebase or force-push)\n"},
		{"--full", true, reviewed, git.ReviewDiff{Diff: "d"},
			"bark: full review (--full)\n"},
		{"nothing new", false, reviewed, git.ReviewDiff{Since: reviewed},
			"bark: nothing changed since the last review at 0123456, so there's nothing to review (--full reviews the whole pull request)\n"},
		{"only ignored files", false, reviewed, git.ReviewDiff{Since: reviewed, Commits: []git.Commit{{}, {}}},
			"bark: the 2 commit(s) since the last review at 0123456 change only files Bark ignores, so there's nothing to review (--full reviews the whole pull request)\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stderr strings.Builder
			noteReviewScope(&stderr, tt.full, tt.recorded, tt.r)
			assert.Equal(t, tt.want, stderr.String())
		})
	}
}

// With nothing to review, the output still says why, so CI can tell it
// from a review that failed to produce anything.
func TestNothingToReview(t *testing.T) {
	t.Parallel()

	reviewed := "0123456789abcdef0123456789abcdef01234567"
	assert.Equal(t, "Nothing to review: nothing changed since the last review at `0123456`.",
		nothingToReview(git.ReviewDiff{Since: reviewed}))
	assert.Equal(t, "Nothing to review: the changes since the last review at `0123456` touch only files Bark ignores.",
		nothingToReview(git.ReviewDiff{Since: reviewed, Commits: []git.Commit{{}}}))
}

// What a posted review records: where the next review starts from.
func TestPostScope(t *testing.T) {
	t.Parallel()

	var r git.ReviewDiff
	r.Since = "0001111"
	assert.Equal(t, prreview.Scope{Since: "0001111"}, postScope(r, false))
	assert.True(t, postScope(r, true).Partial, "the model's output ended early")
	r.Cut = &git.DiffCut{Files: 3, Kept: 1}
	assert.True(t, postScope(r, false).Partial, "the diff was cut")
}

// Reading earlier reviews says only what applies to this review.
func TestHistoryNote(t *testing.T) {
	t.Parallel()

	failed := errors.New("failed to read earlier reviews: HTTP 502")
	two := prreview.History{Findings: []findings.Finding{{}, {}}}
	tests := []struct {
		name                  string
		h                     prreview.History
		err                   error
		checking, incremental bool
		want                  string
	}{
		{"failed, both", prreview.History{}, failed, true, true,
			"bark: failed to read earlier reviews: HTTP 502; repeats of earlier findings won't be set aside, and the whole pull request is reviewed\n"},
		{"failed, no check", prreview.History{}, failed, false, true,
			"bark: failed to read earlier reviews: HTTP 502; the whole pull request is reviewed\n"},
		{"failed, not incremental", prreview.History{}, failed, true, false,
			"bark: failed to read earlier reviews: HTTP 502; repeats of earlier findings won't be set aside\n"},
		{"findings, check", two, nil, true, true,
			"bark: earlier reviews of this pull request raised 2 finding(s)\n"},
		{"findings, no check", two, nil, false, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stderr strings.Builder
			historyNote(&stderr, tt.h, tt.err, tt.checking, tt.incremental)
			assert.Equal(t, tt.want, stderr.String())
		})
	}
}

// The gate fails on findings at or above fail_on that the check didn't
// doubt, repeats of points raised before included, and says on stderr which
// ones.
func TestGate(t *testing.T) {
	t.Parallel()

	events := []findings.Event{
		{Finding: &findings.Finding{File: "internal/x.go", Line: 42, Severity: findings.Critical, Title: "Nil map write"}},
		{Finding: &findings.Finding{File: "cmd/y.go", Line: 10, Severity: findings.High, Title: "Missing timeout"}},
		{Finding: &findings.Finding{File: "cmd/z.go", Line: 3, Severity: findings.High, Title: "Doubted", Doubt: "No."}},
		{Finding: &findings.Finding{File: "cmd/r.go", Line: 4, Severity: findings.Critical, Title: "Repeat", RaisedBefore: "x"}},
		{Finding: &findings.Finding{Severity: findings.High, Title: "No file"}},
		{Finding: &findings.Finding{File: "go.mod", Severity: findings.Low, Title: "Low one"}},
	}

	t.Run("fails", func(t *testing.T) {
		t.Parallel()

		var w strings.Builder
		err := gate(&w, events, findings.High)
		var failErr *FailOnError
		require.ErrorAs(t, err, &failErr)
		assert.Len(t, failErr.Findings, 4)
		assert.Equal(t, findings.High, failErr.Threshold)
		assert.Equal(t, "4 finding(s) at or above high severity (fail_on high)", err.Error())
		assert.Equal(t, "bark: failing: 4 finding(s) at or above high severity (fail_on high):\n"+
			"bark:   [critical] internal/x.go:42 Nil map write\n"+
			"bark:   [high] cmd/y.go:10 Missing timeout\n"+
			"bark:   [critical] cmd/r.go:4 Repeat (raised before, still there)\n"+
			"bark:   [high] No file\n", w.String())
	})

	t.Run("passes", func(t *testing.T) {
		t.Parallel()

		var w strings.Builder
		require.NoError(t, gate(&w, events[2:3], findings.Low), "doubted findings don't count")
		assert.Empty(t, w.String())
		require.NoError(t, gate(&w, events, ""), "no threshold")
		assert.Empty(t, w.String())
	})
}

// With --comment, the review is posted before the gate fails the run, and a
// failed post wins over the gate.
func TestConcludeReview(t *testing.T) {
	t.Parallel()

	events := []findings.Event{{Finding: &findings.Finding{File: "a.go", Line: 2, Severity: findings.High, Title: "Nil map"}}}

	t.Run("posts, then fails", func(t *testing.T) {
		t.Parallel()

		var w strings.Builder
		posted := false
		err := concludeReview(&w, events, findings.High, func() error {
			assert.Empty(t, w.String(), "the review is posted before the gate reports")
			posted = true
			return nil
		})
		assert.True(t, posted)
		var failErr *FailOnError
		require.ErrorAs(t, err, &failErr)
		assert.Contains(t, w.String(), "Nil map")
	})

	t.Run("no post", func(t *testing.T) {
		t.Parallel()

		var w strings.Builder
		var failErr *FailOnError
		require.ErrorAs(t, concludeReview(&w, events, findings.High, nil), &failErr)
		require.NoError(t, concludeReview(&w, events, findings.Critical, nil))
	})

	t.Run("failed post", func(t *testing.T) {
		t.Parallel()

		var w strings.Builder
		postErr := errors.New("gh failed")
		err := concludeReview(&w, events, findings.High, func() error { return postErr })
		require.ErrorIs(t, err, postErr)
		var failErr *FailOnError
		assert.NotErrorAs(t, err, &failErr, "a post failure is a Bark error, not a gate failure")
	})
}

// fail_on below min_severity can never see the findings in between.
func TestWarnFailOnBelowMin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		failOn, min findings.Severity
		want        string
	}{
		{findings.Low, findings.High, "bark: warning: fail_on low is below min_severity high; findings below high are left out, so only those at high or above can fail the review\n"},
		{findings.High, findings.High, ""},
		{findings.Critical, findings.High, ""},
		{findings.Low, "", ""},
		{"", findings.High, ""},
	}
	for _, tt := range tests {
		var w strings.Builder
		warnFailOnBelowMin(&w, tt.failOn, tt.min)
		assert.Equal(t, tt.want, w.String(), "fail_on %q, min_severity %q", tt.failOn, tt.min)
	}
}

// "Kept" means not doubted, as for --fail-on: repeats of points raised
// before are kept, just not posted again.
func TestCheckSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		res  check.Result
		want string
	}{
		{check.Result{Checked: 3, Doubted: 1}, "bark: the check kept 2 of 3 finding(s)\n"},
		{check.Result{Checked: 2, RaisedBefore: 2}, "bark: the check kept 2 of 2 finding(s); 2 raised before on this pull request, not posted again\n"},
		{check.Result{Checked: 4, Doubted: 1, RaisedBefore: 2}, "bark: the check kept 3 of 4 finding(s); 2 raised before on this pull request, not posted again\n"},
		{check.Result{Checked: 1, Doubted: 1, Unchecked: 2}, "bark: the check kept 0 of 1 finding(s); it gave no verdict on 2 finding(s), they're unchecked\n"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, checkSummary(tt.res))
	}
}

func TestSkipPost(t *testing.T) {
	t.Parallel()

	const head = "fc4cb814c9a4b110f6b03b5f77182099e9f1dfee"
	last := prreview.History{Head: head}
	scope := prreview.Scope{Head: head}
	repeat := findings.Event{Finding: &findings.Finding{File: "a.go", Line: 2, Title: "Again", RaisedBefore: "Earlier"}}

	t.Run("only repeats", func(t *testing.T) {
		t.Parallel()

		var w strings.Builder
		assert.True(t, skipPost(&w, []findings.Event{repeat}, scope, last))
		assert.Equal(t, "bark: not posted: nothing new since the last review at fc4cb81: "+
			"every finding repeats a point raised before or dismissed\n", w.String())
	})

	t.Run("no findings", func(t *testing.T) {
		t.Parallel()

		var w strings.Builder
		assert.True(t, skipPost(&w, []findings.Event{{Verdict: "Fine."}}, scope, last))
		assert.Equal(t, "bark: not posted: nothing new since the last review at fc4cb81: no findings\n", w.String())
	})

	t.Run("something new", func(t *testing.T) {
		t.Parallel()

		var w strings.Builder
		events := []findings.Event{repeat, {Finding: &findings.Finding{File: "a.go", Line: 3, Title: "New"}}}
		assert.False(t, skipPost(&w, events, scope, last))
		assert.Empty(t, w.String())
	})
}
