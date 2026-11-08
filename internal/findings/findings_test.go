package findings

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   []Event
	}{
		{
			name:   "finding and verdict",
			output: `{"file":"a.go","line":3,"end_line":5,"severity":"high","category":"bug","title":"T","body":"B","suggestion":"x := 1","remark":"R"}` + "\n" + `{"type":"verdict","text":" Fine. "}` + "\n",
			want: []Event{
				{Finding: &Finding{File: "a.go", Line: 3, EndLine: 5, Severity: High, Category: "bug", Title: "T", Body: "B", Suggestion: "x := 1", Remark: "R"}},
				{Verdict: "Fine."},
			},
		},
		{
			name:   "intro, finding and verdict",
			output: `{"type":"intro","text":" What is this? "}` + "\n" + `{"file":"a.go","severity":"low","title":"T"}` + "\n" + `{"type":"verdict","text":"No."}`,
			want: []Event{
				{Intro: "What is this?"},
				{Finding: &Finding{File: "a.go", Severity: Low, Title: "T"}},
				{Verdict: "No."},
			},
		},
		{
			name:   "empty intro is skipped",
			output: `{"type":"intro","text":""}`,
			want:   nil,
		},
		{
			name:   "last line without newline",
			output: `{"type":"verdict","text":"ok"}`,
			want:   []Event{{Verdict: "ok"}},
		},
		{
			name:   "code fences and blank lines are skipped",
			output: "```jsonl\n\n" + `{"type":"verdict","text":"ok"}` + "\r\n```\n",
			want:   []Event{{Verdict: "ok"}},
		},
		{
			name:   "severity is lowercased, unknown values are kept and a missing one becomes medium",
			output: `{"file":"a.go","severity":"HIGH","title":"A"}` + "\n" + `{"file":"a.go","severity":" Nuclear ","title":"B"}` + "\n" + `{"file":"a.go","title":"C"}`,
			want: []Event{
				{Finding: &Finding{File: "a.go", Severity: High, Title: "A"}},
				{Finding: &Finding{File: "a.go", Severity: "nuclear", Title: "B"}},
				{Finding: &Finding{File: "a.go", Severity: Medium, Title: "C"}},
			},
		},
		{
			name:   "end_line not after line is dropped",
			output: `{"file":"a.go","line":4,"end_line":4,"severity":"low","title":"A"}`,
			want:   []Event{{Finding: &Finding{File: "a.go", Line: 4, Severity: Low, Title: "A"}}},
		},
		{
			name:   "empty verdict is skipped",
			output: `{"type":"verdict","text":"  "}`,
			want:   nil,
		},
		{
			name:   "free text is kept as text",
			output: "## Review\nLooks fine to me.\r\n",
			want:   []Event{{Text: "## Review"}, {Text: "Looks fine to me."}},
		},
		{
			name:   "invalid JSON is kept as text",
			output: `{"file":"a.go","title":` + "\n",
			want:   []Event{{Text: `{"file":"a.go","title":`}},
		},
		{
			name:   "object without title or body is kept as text",
			output: `{"foo":1}`,
			want:   []Event{{Text: `{"foo":1}`}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, Parse(tt.output))
		})
	}
}

// Chunk boundaries must not matter: feeding the output in any split gives the
// same events as parsing it whole.
func TestParserFeedAcrossChunks(t *testing.T) {
	t.Parallel()

	output := `{"file":"a.go","line":1,"severity":"low","title":"A","body":"x"}` + "\n" +
		"stray text\n" +
		`{"type":"verdict","text":"done"}`
	want := Parse(output)
	require.Len(t, want, 3)

	for size := 1; size <= len(output); size++ {
		var p Parser
		var got []Event
		for i := 0; i < len(output); i += size {
			got = append(got, p.Feed(output[i:min(i+size, len(output))])...)
		}
		got = append(got, p.Flush()...)
		require.Equal(t, want, got, "chunk size %d", size)
	}
}

// A line is reported as soon as its newline arrives, not at the end.
func TestParserFeedEmitsCompleteLines(t *testing.T) {
	t.Parallel()

	var p Parser
	assert.Empty(t, p.Feed(`{"file":"a.go","severity":"low",`))
	assert.Equal(t, []Event{{Finding: &Finding{File: "a.go", Severity: Low, Title: "A"}}}, p.Feed(`"title":"A"}`+"\n{"))
	assert.Equal(t, []Event{{Text: "{"}}, p.Flush())
	assert.Empty(t, p.Flush())
}

func TestMarkdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		events []Event
		want   string
	}{
		{
			name: "full finding",
			events: []Event{{Finding: &Finding{
				File: "cmd/review.go", Line: 3, EndLine: 4, Severity: High, Category: "bug",
				Title: "Nil map write", Body: "m is nil.", Suggestion: "m := map[string]int{}\n", Remark: "Illogical.",
			}}},
			want: "### [HIGH · bug] Nil map write\n\n" +
				"`cmd/review.go:3-4`\n\n" +
				"m is nil.\n\n" +
				"```go\nm := map[string]int{}\n```\n\n" +
				"> _Illogical._\n\n",
		},
		{
			name:   "finding without line or category",
			events: []Event{{Finding: &Finding{File: "README", Severity: Low, Title: "Typo"}}},
			want:   "### [LOW] Typo\n\n`README`\n\n",
		},
		{
			name:   "suggestion containing a fence gets a longer fence",
			events: []Event{{Finding: &Finding{File: "a.md", Line: 1, Severity: Low, Title: "T", Suggestion: "```go\n```"}}},
			want:   "### [LOW] T\n\n`a.md:1`\n\n````markdown\n```go\n```\n````\n\n",
		},
		{
			name:   "intro",
			events: []Event{{Intro: "Oh dear.\n\nWhere to start."}},
			want:   "Oh dear.\n\nWhere to start.\n\n",
		},
		{
			name:   "unknown severity is shown as written",
			events: []Event{{Finding: &Finding{File: "a.go", Severity: "nuclear", Category: "bug", Title: "T"}}},
			want:   "### [NUCLEAR · bug] T\n\n`a.go`\n\n",
		},
		{
			name:   "verdict and text",
			events: []Event{{Text: "raw"}, {Verdict: "Ship it."}},
			want:   "raw\n## Verdict\n\nShip it.\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, Markdown(tt.events))
		})
	}
}

// Rendering batch by batch must equal rendering everything at once, since the
// TUI and --stream append each batch as it arrives.
func TestMarkdownIsAppendable(t *testing.T) {
	t.Parallel()

	events := []Event{
		{Intro: "i"},
		{Finding: &Finding{File: "a.go", Line: 1, Severity: Low, Title: "A"}},
		{Text: "x"},
		{Verdict: "v"},
	}
	var b strings.Builder
	for _, e := range events {
		b.WriteString(Markdown([]Event{e}))
	}
	assert.Equal(t, Markdown(events), b.String())
}
