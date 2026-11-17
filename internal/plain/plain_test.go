package plain

import (
	"context"
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

	cut := &git.DiffCut{Files: 5, Kept: 2, Setting: "max_diff_tokens"}
	note := partialReviewNote(cut)
	assert.Equal(t, "> **Partial review:** Bark reviewed only the first 2 of 5 changed files: the diff was cut to fit max_diff_tokens.\n", note)

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
