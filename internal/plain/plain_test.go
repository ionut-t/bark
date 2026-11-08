package plain

import (
	"context"
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeLLM returns output whole from Generate, and from Stream in chunks of
// chunkSize bytes, so lines are split across chunks.
type fakeLLM struct {
	output    string
	chunkSize int
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
	}()
	return out, errc
}

func (f fakeLLM) Generate(context.Context, string, string) (llm.Response, error) {
	return llm.Response{Content: f.output}, nil
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
				raw, err = streamResponse(context.Background(), client, "", "", &out, tt.structured, nil)
			} else {
				raw, err = fullResponse(context.Background(), client, "", "", nil)
			}
			require.NoError(t, err)
			finishReview(&out, raw, tt.stream, tt.structured, nil)
			assert.Equal(t, tt.want, out.String())
			assert.Equal(t, structuredOutput, raw)
		})
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
		events := finishReview(&out, structuredOutput, false, true, doubtAll)

		assert.Equal(t, "## Doubted by Bark's check\n\n"+
			"A second pass checked these findings against the full files and doubts them. They are listed in case it's wrong.\n\n"+
			doubted+"## Verdict\n\nFix the map.\n\n\n", out.String())
		require.Len(t, events, 2)
		assert.Equal(t, "m is never nil.", events[0].Finding.Doubt)
	})

	t.Run("streamed", func(t *testing.T) {
		t.Parallel()

		var out strings.Builder
		finishReview(&out, structuredOutput, true, true, doubtAll)

		assert.Equal(t, "## Bark's check\n\nA second pass checked 1 finding against the full files and doubts 1:\n\n"+doubted, out.String())
	})

	t.Run("free text is never checked", func(t *testing.T) {
		t.Parallel()

		var out strings.Builder
		events := finishReview(&out, "plain text", false, false, func([]findings.Event) ([]findings.Event, int) {
			t.Fatal("free text has no findings to check")
			return nil, 0
		})
		assert.Nil(t, events)
		assert.Equal(t, "plain text\n", out.String())
	})
}
