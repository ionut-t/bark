package plain

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ionut-t/bark/v2/internal/check"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/ionut-t/bark/v2/internal/prices"
	"github.com/ionut-t/bark/v2/internal/prreview"
	"github.com/ionut-t/bark/v2/internal/redact"
	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/ionut-t/bark/v2/internal/tokens"
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
			finishReview(&out, raw, false, tt.stream, tt.structured, "", nil, nil)
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
			events := finishReview(&out, raw, truncated, tt.stream, tt.structured, "", nil, nil)
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
		}, nil)
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
		events := finishReview(&out, structuredOutput, false, false, true, "", doubtAll, nil)

		assert.Equal(t, "## Doubted by Bark's check\n\n"+
			"A second pass checked these findings against the code and doubts them. They are listed in case it's wrong.\n\n"+
			doubted+"## Verdict\n\nFix the map.\n\n\n", out.String())
		require.Len(t, events, 2)
		assert.Equal(t, "m is never nil.", events[0].Finding.Doubt)
	})

	t.Run("streamed", func(t *testing.T) {
		t.Parallel()

		var out strings.Builder
		finishReview(&out, structuredOutput, false, true, true, "", doubtAll, nil)

		assert.Equal(t, "## Bark's check\n\nA second pass checked 1 finding against the code and doubts 1:\n\n"+doubted, out.String())
	})

	t.Run("free text is never checked", func(t *testing.T) {
		t.Parallel()

		var out strings.Builder
		events := finishReview(&out, "plain text", false, false, false, "", func([]findings.Event) ([]findings.Event, int) {
			t.Fatal("free text has no findings to check")
			return nil, 0
		}, nil)
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

// emptyLLM answers every call with nothing, which fails Bark's check.
type emptyLLM struct{}

func (emptyLLM) Stream(context.Context, string, string) (<-chan llm.Response, <-chan error) {
	panic("the check doesn't stream")
}

func (emptyLLM) Generate(context.Context, string, string) (llm.Response, error) {
	return llm.Response{}, nil
}

// When Bark's check fails, every finding is marked unchecked, so a posted
// review doesn't present them as checked.
func TestCheckFindingsFailedMarksUnchecked(t *testing.T) {
	t.Parallel()

	events := []findings.Event{
		{Finding: &findings.Finding{File: "a.go", Line: 1, Severity: findings.High, Title: "A"}},
		{Finding: &findings.Finding{File: "a.go", Line: 2, Severity: findings.Low, Title: "B"}},
		{Verdict: "Fix it."},
	}
	got, checked, _ := checkFindings(check.Review{Client: emptyLLM{}}, events, nil, time.Minute)
	assert.Equal(t, 0, checked)
	assert.Equal(t, []findings.Event{
		{Finding: &findings.Finding{File: "a.go", Line: 1, Severity: findings.High, Title: "A", Unchecked: "it failed."}},
		{Finding: &findings.Finding{File: "a.go", Line: 2, Severity: findings.Low, Title: "B", Unchecked: "it failed."}},
		{Verdict: "Fix it."},
	}, got)
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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// failingGHOnPath puts a gh on PATH that fails every call.
func failingGHOnPath(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as gh")
	}
	bin := t.TempDir()
	writeFile(t, filepath.Join(bin, "gh"), "#!/bin/sh\necho 'gh: boom' >&2\nexit 1\n")
	require.NoError(t, os.Chmod(filepath.Join(bin, "gh"), 0o755))
	t.Setenv("PATH", bin)
}

func TestResolveReviewer(t *testing.T) {
	storage := t.TempDir()
	require.NoError(t, reviewers.Config(storage, false))

	t.Run("from a file path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "Mine.md")
		writeFile(t, path, "Be strict.")
		r, err := resolveReviewer(path, storage, false)
		require.NoError(t, err)
		assert.Equal(t, &reviewers.Reviewer{Name: "Mine", Prompt: "Be strict."}, r)
	})

	t.Run("by name", func(t *testing.T) {
		t.Chdir(t.TempDir())
		r, err := resolveReviewer("linus", storage, false)
		require.NoError(t, err)
		assert.Equal(t, "Linus Torvalds", r.Name)
	})

	// A reviewer picked from preferred_reviewers is a name: a repository's
	// config mustn't be able to read a local file into the prompt.
	t.Run("picked by name ignores a file path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "Mine.md")
		writeFile(t, path, "Private notes.")
		_, err := resolveReviewer(path, storage, true)
		assert.ErrorContains(t, err, "reviewer not found")
	})

	t.Run("picked by name", func(t *testing.T) {
		t.Chdir(t.TempDir())
		writeFile(t, "linus", "Not a persona.")
		r, err := resolveReviewer("linus", storage, true)
		require.NoError(t, err)
		assert.Equal(t, "Linus Torvalds", r.Name)
	})

	t.Run("unknown name", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_, err := resolveReviewer("nobody-at-all", storage, false)
		assert.ErrorContains(t, err, "reviewer not found")
	})

	t.Run("missing storage", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_, err := resolveReviewer("linus", filepath.Join(t.TempDir(), "missing"), false)
		assert.ErrorContains(t, err, "error loading reviewers")
	})

	t.Run("repo reviewer file", func(t *testing.T) {
		t.Chdir(t.TempDir())
		writeFile(t, filepath.Join(".bark", "reviewer.md"), "Repo persona.")
		r, err := resolveReviewer("", storage, false)
		require.NoError(t, err)
		assert.Equal(t, &reviewers.Reviewer{Name: "reviewer", Prompt: "Repo persona."}, r)
	})

	t.Run("none given", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_, err := resolveReviewer("", storage, false)
		assert.ErrorContains(t, err, "reviewer is required in plain mode")
	})

	// Run from a subdirectory, the repository's reviewer is still found
	// at its root.
	t.Run("repo reviewer file from a subdirectory", func(t *testing.T) {
		root := t.TempDir()
		t.Chdir(root)
		require.NoError(t, exec.Command("git", "init", "-q").Run())
		writeFile(t, filepath.Join(root, ".bark", "reviewer.md"), "Repo persona.")
		sub := filepath.Join(root, "internal", "pkg")
		require.NoError(t, os.MkdirAll(sub, 0o755))
		t.Chdir(sub)

		r, err := resolveReviewer("", storage, false)
		require.NoError(t, err)
		assert.Equal(t, "Repo persona.", r.Prompt)
	})

	t.Run("repo reviewer path is not a file", func(t *testing.T) {
		t.Chdir(t.TempDir())
		require.NoError(t, os.MkdirAll(filepath.Join(".bark", "reviewer.md"), 0o755))

		_, err := resolveReviewer("", storage, false)
		assert.ErrorContains(t, err, "is not a file")
	})
}

func TestResolveInstructions(t *testing.T) {
	t.Chdir(t.TempDir())
	storage := t.TempDir()

	got, err := resolveInstructions(nil, storage)
	require.NoError(t, err)
	assert.Empty(t, got, "no values and no .bark/review.md")

	writeFile(t, filepath.Join(".bark", "review.md"), "Repo review rules.")
	got, err = resolveInstructions(nil, storage)
	require.NoError(t, err)
	assert.Equal(t, "Repo review rules.", got)

	got, err = resolveInstructions([]string{"Be brief."}, storage)
	require.NoError(t, err)
	assert.Equal(t, "Be brief.", got, "values win over the repo file")

	writeFile(t, filepath.Join(storage, "instructions", "Security.md"), "Look for injections.")
	got, err = resolveInstructions([]string{"security"}, storage)
	require.NoError(t, err)
	assert.Equal(t, "Look for injections.", got, "saved instructions are looked up by name")
}

func TestResolvePRInstructions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	cfg := config.New("", config.Data{})

	got, err := resolvePRInstructions(nil, cfg)
	require.NoError(t, err)
	assert.Equal(t, cfg.GetPRInstructions(), got, "the configured instructions by default")

	writeFile(t, filepath.Join(".bark", "pr.md"), "Repo PR rules.")
	got, err = resolvePRInstructions(nil, cfg)
	require.NoError(t, err)
	assert.Equal(t, "Repo PR rules.", got)

	got, err = resolvePRInstructions([]string{"Short, please."}, cfg)
	require.NoError(t, err)
	assert.Equal(t, "Short, please.", got)
}

type nopLLM struct{}

func (nopLLM) Stream(context.Context, string, string) (<-chan llm.Response, <-chan error) {
	return nil, nil
}

func (nopLLM) Generate(context.Context, string, string) (llm.Response, error) {
	return llm.Response{}, nil
}

func TestMeterCalls(t *testing.T) {
	t.Chdir(t.TempDir())
	cache := t.TempDir()
	client := nopLLM{}

	got, meter := meterCalls(client, cache, "gemini", "gemini-3.6-flash")
	assert.Equal(t, client, got, "no prices file: the client is unwrapped")
	assert.Nil(t, meter)

	got, meter = meterCalls(client, cache, "no-such-provider", "x")
	assert.Equal(t, client, got)
	assert.Nil(t, meter)

	require.NoError(t, prices.Save(filepath.Join(cache, prices.FileName), prices.File{
		Date:      "2026-11-01",
		Providers: map[string]map[string]prices.Price{"google": {"gemini-3.6-flash": {Input: 1, Output: 2}}},
	}))

	got, meter = meterCalls(client, cache, "gemini", "gemini-3.6-flash")
	require.NotNil(t, meter)
	assert.Equal(t, meter, got, "a priced model's calls go through the meter")

	got, meter = meterCalls(client, cache, "gemini", "unlisted-model")
	assert.Equal(t, client, got, "an unlisted model is a warning, not an error")
	assert.Nil(t, meter)
}

func TestLineMapperWithoutHead(t *testing.T) {
	assert.Nil(t, lineMapper(newLineMaps("")))
}

func TestLineMapperReportsEachFailureOnce(t *testing.T) {
	t.Chdir(t.TempDir()) // not a git repository: every lookup fails
	m := lineMapper(newLineMaps("HEAD"))
	require.NotNil(t, m)

	_, _, ok, err := m(postedOn, "a.go", 1, 2)
	assert.False(t, ok)
	assert.Error(t, err)

	_, _, ok, err = m(postedOn, "a.go", 3, 4)
	assert.False(t, ok)
	assert.NoError(t, err, "a failure already reported is not reported again")

	_, _, _, err = m(postedOn, "b.go", 1, 1)
	assert.Error(t, err, "another file's failure is reported")
}

func TestReadHistoryFailure(t *testing.T) {
	failingGHOnPath(t)
	h, ok := readHistory("7", true, true)
	assert.False(t, ok)
	assert.Empty(t, h.Findings)
}

func TestPartialReviewNoteNil(t *testing.T) {
	assert.Empty(t, partialReviewNote(nil))
}

// captureStderr returns what f writes to os.Stderr.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	return capture(t, &os.Stderr, f)
}

// capture returns what f writes to *file (os.Stdout or os.Stderr).
func capture(t *testing.T, file **os.File, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	old := *file
	*file = w
	defer func() { *file = old }()

	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	f()
	require.NoError(t, w.Close())
	return <-done
}

// usageLLM answers every call with usage, so a meter has something to
// count.
type usageLLM struct{ nopLLM }

func (usageLLM) Generate(context.Context, string, string) (llm.Response, error) {
	return llm.Response{Usage: &llm.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000, TotalTokens: 2_000_000}}, nil
}

func TestWarnTruncated(t *testing.T) {
	got := captureStderr(t, func() { warnTruncated("review", "it may be missing findings") })
	assert.Equal(t, "bark: warning: the model reached its output limit, so the review was cut off; it may be missing findings\n", got)
}

func TestPrintCost(t *testing.T) {
	assert.Empty(t, captureStderr(t, func() { printCost(nil) }), "no meter, no cost")

	meter := prices.NewMeter(usageLLM{}, &prices.Pricing{Price: prices.Price{Input: 1, Output: 2}, Date: "2026-11-01"})
	assert.Empty(t, captureStderr(t, func() { printCost(meter) }), "no calls, no cost")

	_, err := meter.Generate(context.Background(), "s", "p")
	require.NoError(t, err)
	got := captureStderr(t, func() { printCost(meter) })
	assert.Contains(t, got, "bark: ")
	assert.Contains(t, got, "$3.00")
}

// isolateProvider clears every provider's credentials and points Bark's
// directories at a fresh temporary tree. It returns the cache directory.
func isolateProvider(t *testing.T) string {
	t.Helper()
	for _, key := range []string{"GEMINI_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "OLLAMA_HOST", "VERTEXAI_PROJECT_ID", "VERTEXAI_LOCATION", "BARK_DEMO_MOCK"} {
		t.Setenv(key, "")
	}
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Chdir(t.TempDir())
	return filepath.Join(root, "cache", "bark")
}

func TestNewMeteredClient(t *testing.T) {
	cfg := func() config.Config {
		return config.New("", config.Data{LLMProvider: "gemini", LLMModel: "gemini-3.6-flash"})
	}

	t.Run("no credentials", func(t *testing.T) {
		isolateProvider(t)
		_, _, err := newMeteredClient(cfg())
		assert.ErrorContains(t, err, "error creating LLM client")
	})

	t.Run("unpriced model has no meter", func(t *testing.T) {
		isolateProvider(t)
		t.Setenv("GEMINI_API_KEY", "fake")
		client, meter, err := newMeteredClient(cfg())
		require.NoError(t, err)
		assert.NotNil(t, client)
		assert.Nil(t, meter)
	})

	t.Run("priced model is metered", func(t *testing.T) {
		cache := isolateProvider(t)
		t.Setenv("GEMINI_API_KEY", "fake")
		require.NoError(t, prices.Save(filepath.Join(cache, prices.FileName), prices.File{
			Date:      "2026-11-01",
			Providers: map[string]map[string]prices.Price{"google": {"gemini-3.6-flash": {Input: 1, Output: 2}}},
		}))

		client, meter, err := newMeteredClient(cfg())
		require.NoError(t, err)
		require.NotNil(t, meter)
		assert.Equal(t, meter, client, "calls go through the meter")
	})
}

// llm_timeout limits a commit message or PR description call: a model that
// never answers fails the run once the timeout passes, not after the
// 3-minute default.
func TestRunCommitAndPR_LLMTimeout(t *testing.T) {
	tests := []struct {
		name string
		run  func(cfg config.Config, diff *string) error
	}{
		{name: "commit", run: func(cfg config.Config, diff *string) error {
			return RunCommit(CommitOptions{Diff: diff, Config: cfg})
		}},
		{name: "pr", run: func(cfg config.Config, diff *string) error {
			return RunPR(PROptions{Diff: diff, Config: cfg})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateProvider(t)
			// An Ollama server that never answers.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// The server only notices the client hanging up once the
				// body is read.
				_, _ = io.Copy(io.Discard, r.Body)
				<-r.Context().Done()
			}))
			t.Cleanup(srv.Close)
			t.Setenv("OLLAMA_HOST", srv.URL)

			cfg := config.New("", config.Data{LLMProvider: "ollama", LLMModel: "llama3", LLMTimeout: "200ms"})
			diff := "diff --git a/a.go b/a.go\n+x\n"
			start := time.Now()
			err := tt.run(cfg, &diff)
			require.Error(t, err)
			assert.ErrorIs(t, err, context.DeadlineExceeded)
			assert.Less(t, time.Since(start), 30*time.Second)
		})
	}
}

func TestDryRun(t *testing.T) {
	const key = "ghp_" + "abcdefghijklmnopqrstuvwxyz0123456789"
	system := "You are a reviewer.\n"
	promptText := "**Code to review:**\n+token := \"" + key + "\"\n"
	pricing := &prices.Pricing{Price: prices.Price{Rates: prices.Rates{Input: 1_000_000}}, Date: "2026-11-01"}

	var out, log strings.Builder
	dryRun(&out, &log, system, promptText, nil, "", true, pricing, "")

	assert.Equal(t, dryRunSystemHeading+"\nYou are a reviewer.\n\n"+dryRunPromptHeading+"\n**Code to review:**\n+token := \"[REDACTED:github-token]\"\n\n"+redact.Note+"\n", out.String(),
		"the prompts are shown masked, as they would be sent")
	assert.NotContains(t, out.String(), key)

	stderr := log.String()
	assert.Contains(t, stderr, "masked 1 secret(s)")
	assert.Contains(t, stderr, "no model call made")
	sentSystem, sentPrompt, _ := redact.Prompts(system, promptText)
	input := tokens.Estimate(len(sentSystem)) + tokens.Estimate(len(sentPrompt))
	require.Positive(t, input)
	assert.Contains(t, stderr, fmt.Sprintf("about %d input tokens", input), "the masked prompts are counted")
	assert.Contains(t, stderr, "that input would cost about "+prices.FormatUSD(float64(input))+" (prices from 2026-11-01)",
		"priced at $1 a token, the cost is the token count")
	assert.Contains(t, stderr, "the check's call aren't counted")
	assert.NotContains(t, stderr, "warning: Bark reviewed only")
	assert.NotContains(t, stderr, "over the 800-line cap")
}

func TestDryRunWithoutPricing(t *testing.T) {
	var out, log strings.Builder
	cut := &git.DiffCut{Files: 3, Kept: 1, Setting: "max_diff_tokens"}
	dryRun(&out, &log, "system", "prompt", cut, "2 enclosing declarations (130 lines) over the 800-line cap left out\n  b.go f: 100 lines, 3 changed\n  c.go g: 30 lines, 1 changed", false, nil, "no prices file; run bark prices to download one")

	stderr := log.String()
	assert.Contains(t, stderr, "bark: warning: "+cut.Summary())
	assert.Contains(t, stderr, "bark: dry run: 2 enclosing declarations (130 lines) over the 800-line cap left out\n  b.go f: 100 lines, 3 changed\n  c.go g: 30 lines, 1 changed\n")
	assert.Contains(t, stderr, "bark: dry run: no cost estimate: no prices file; run bark prices to download one")
	assert.NotContains(t, stderr, "would cost")
}

func TestDryRunPricing(t *testing.T) {
	cfg := func(data config.Data) config.Config {
		c := config.New("", data)
		require.NoError(t, c.OverrideProvider(""))
		return c
	}

	t.Run("priced without credentials", func(t *testing.T) {
		cache := isolateProvider(t)
		require.NoError(t, prices.Save(filepath.Join(cache, prices.FileName), prices.File{
			Date:      "2026-11-01",
			Providers: map[string]map[string]prices.Price{"google": {"gemini-3.6-flash": {Rates: prices.Rates{Input: 1}}}},
		}))
		pricing, why := dryRunPricing(cfg(config.Data{LLMProvider: "gemini", LLMModel: "gemini-3.6-flash"}), cache)
		require.NotNil(t, pricing, why)
		assert.Equal(t, "2026-11-01", pricing.Date)
	})

	t.Run("no prices file", func(t *testing.T) {
		cache := isolateProvider(t)
		pricing, why := dryRunPricing(cfg(config.Data{LLMProvider: "gemini", LLMModel: "gemini-3.6-flash"}), cache)
		assert.Nil(t, pricing)
		assert.Contains(t, why, "bark prices")
	})

	t.Run("ollama", func(t *testing.T) {
		cache := isolateProvider(t)
		pricing, why := dryRunPricing(cfg(config.Data{LLMProvider: "ollama", LLMModel: "llama3"}), cache)
		assert.Nil(t, pricing)
		assert.Contains(t, why, "Ollama")
	})

	t.Run("no model", func(t *testing.T) {
		cache := isolateProvider(t)
		pricing, why := dryRunPricing(cfg(config.Data{}), cache)
		assert.Nil(t, pricing)
		assert.NotEmpty(t, why)
	})
}

// A dry run returns before the model client is made, so it works without
// credentials, and with --comment posts nothing.
func TestRunReviewDryRunMakesNoCall(t *testing.T) {
	isolateProvider(t)
	require.NoError(t, os.MkdirAll(".bark", 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(".bark", "reviewer.md"), []byte("You are a reviewer."), 0o644))
	diff := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-a\n+b\n"
	cfg := config.New("", config.Data{LLMProvider: "gemini", LLMModel: "gemini-3.6-flash", StructuredFindings: true})

	var err error
	var stderr string
	stdout := capture(t, &os.Stdout, func() {
		stderr = captureStderr(t, func() {
			err = RunReview(ReviewOptions{Diff: &diff, Config: cfg, SkipInstruction: true, DryRun: true})
		})
	})
	require.NoError(t, err, "no credentials are set, so any model client would fail")
	assert.Contains(t, stdout, dryRunSystemHeading)
	assert.Contains(t, stdout, dryRunPromptHeading)
	assert.Contains(t, stdout, "+b", "the diff is in the prompt")
	assert.Contains(t, stderr, "no model call made")
	assert.NotContains(t, stderr, "debug log")
}

// A change to only files Bark ignores (say, a release that only edits
// CHANGELOG.md) has nothing to review, which isn't an error: the review
// says so and succeeds. With no change at all it still fails.
func TestRunReviewIgnoredOnlyChange(t *testing.T) {
	isolateProvider(t)
	gitRun := func(args ...string) {
		out, err := exec.Command("git", args...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	gitRun("init", "-b", "main")
	gitRun("config", "user.name", "Test")
	gitRun("config", "user.email", "test@example.com")
	require.NoError(t, os.WriteFile(".barkignore", []byte("CHANGELOG.md\n"), 0o644))
	gitRun("add", ".barkignore")
	gitRun("commit", "-m", "init")
	cfg := config.New("", config.Data{})

	err := RunReview(ReviewOptions{Config: cfg, SkipInstruction: true})
	assert.ErrorContains(t, err, "no uncommitted changes to review", "nothing changed")

	require.NoError(t, os.WriteFile("CHANGELOG.md", []byte("# 1.0\n"), 0o644))
	var stderr string
	stdout := capture(t, &os.Stdout, func() {
		stderr = captureStderr(t, func() {
			err = RunReview(ReviewOptions{Config: cfg, SkipInstruction: true})
		})
	})
	require.NoError(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, "bark: the uncommitted changes touch only files Bark ignores (CHANGELOG.md), so there's nothing to review\n", stderr)

	gitRun("add", "CHANGELOG.md")
	gitRun("commit", "-m", "release")
	stderr = captureStderr(t, func() {
		err = RunReview(ReviewOptions{Config: cfg, SkipInstruction: true, Hash: "HEAD"})
	})
	require.NoError(t, err)
	assert.Equal(t, "bark: commit HEAD changes only files Bark ignores (CHANGELOG.md), so there's nothing to review\n", stderr)
}

func TestIgnoredOnlyNote(t *testing.T) {
	files := []string{"CHANGELOG.md"}
	tests := []struct {
		name string
		opts ReviewOptions
		r    git.ReviewDiff
		want string
	}{
		{name: "pull request", opts: ReviewOptions{PR: "179"}, r: git.ReviewDiff{Ignored: files},
			want: "pull request #179 changes only files Bark ignores (CHANGELOG.md)"},
		{name: "branch", opts: ReviewOptions{Branch: "main"}, r: git.ReviewDiff{Ignored: files},
			want: "the commits since main change only files Bark ignores (CHANGELOG.md)"},
		{name: "staged", opts: ReviewOptions{Staged: true}, r: git.ReviewDiff{Ignored: files},
			want: "the staged changes touch only files Bark ignores (CHANGELOG.md)"},
		{name: "many files", opts: ReviewOptions{Hash: "abc123"}, r: git.ReviewDiff{Ignored: []string{"a.lock", "b.lock", "c.lock", "d.lock", "e.lock"}},
			want: "commit abc123 changes only files Bark ignores (a.lock, b.lock, c.lock and 2 more)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, "bark: "+tt.want+", so there's nothing to review", ignoredOnlyNote(tt.opts, tt.r))
		})
	}
}

// An empty diff says why for its source, so a branch diff with only
// uncommitted work doesn't fail with a bare "no diff content available".
func TestEmptyDiffError(t *testing.T) {
	stdin := ""
	tests := []struct {
		name string
		opts ReviewOptions
		r    git.ReviewDiff
		want []string
	}{
		{name: "stdin", opts: ReviewOptions{Diff: &stdin}, want: []string{"the diff from stdin is empty"}},
		{name: "branch without commits", opts: ReviewOptions{Branch: "main"},
			want: []string{"HEAD has no commits that main doesn't", "uncommitted changes aren't part of a branch diff", "--changes"}},
		{name: "branch with commits", opts: ReviewOptions{Branch: "main"}, r: git.ReviewDiff{Commits: []git.Commit{{}}},
			want: []string{"the commits since main change no file Bark reviews", ".barkignore"}},
		{name: "commit", opts: ReviewOptions{Hash: "abc123"}, want: []string{"commit abc123 changes no file Bark reviews", ".barkignore"}},
		{name: "pull request", opts: ReviewOptions{PR: "42"}, want: []string{"pull request #42 changes no file Bark reviews", ".barkignore"}},
		{name: "staged", opts: ReviewOptions{Staged: true}, want: []string{"no staged changes to review", "--changes"}},
		{name: "working tree", opts: ReviewOptions{All: true}, want: []string{"no uncommitted changes to review", "--branch"}},
		{name: "default source", want: []string{"no uncommitted changes to review"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := emptyDiffError(tt.opts, tt.r)
			require.Error(t, err)
			for _, want := range tt.want {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}
