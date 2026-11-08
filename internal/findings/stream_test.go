package findings

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renderInChunks streams output through a Renderer in chunks of size bytes.
func renderInChunks(output string, size int) string {
	var r Renderer
	var b strings.Builder
	for i := 0; i < len(output); i += size {
		b.WriteString(r.Write(output[i:min(i+size, len(output))]))
	}
	b.WriteString(r.Close())
	return b.String()
}

// Whatever the chunking (including chunks that split escapes and multi-byte
// characters), the streamed Markdown must equal rendering the whole output.
func TestRendererMatchesWholeRender(t *testing.T) {
	t.Parallel()

	output := "```jsonl\n" +
		`{"type":"intro","text":"  Well, well. \"Structured\", they said.\n\nCafé é time. "}` + "\n" +
		`{"file":"a.go","line":4,"end_line":5,"severity":"high","category":"bug","title":"Nil map write","body":"m is nil when\nn == 0.","suggestion":"m := map[string]int{}\n\tm[k]++\n","remark":"Ugh — again."}` + "\n" +
		"\n" +
		`{"file":"README","severity":"LOW","category":"docs","title":"Typo"}` + "\r\n" +
		"some stray text é\n" +
		`{"file":"b.md","line":1,"severity":"low","title":"Fence","suggestion":"` + "```go\\n```" + `"}` + "\n" +
		"```\n" +
		`{"type":"verdict","text":"Fix it. "}`

	want := Markdown(Parse(output))
	require.Contains(t, want, "Café é time.")

	for size := 1; size <= len(output); size++ {
		require.Equal(t, want, renderInChunks(output, size), "chunk size %d", size)
	}
}

// Prose appears while its line is still arriving, not when it completes.
func TestRendererStreamsUnfinishedLines(t *testing.T) {
	t.Parallel()

	var r Renderer
	assert.Equal(t, "Hello wor", r.Write(`{"type":"intro","text":"Hello wor`))
	assert.Equal(t, "ld\n\n", r.Write(`ld"}`))
	assert.Empty(t, r.Write("\n"))

	assert.Equal(t, "", r.Write(`{"file":"a.go","line":4,"severity":"high","category":"bug","ti`))
	assert.Equal(t, "### [HIGH · bug] Nil", r.Write(`tle":"Nil`))
	assert.Equal(t, " map\n\n`a.go:4`\n\nm is", r.Write(` map","body":"m is`))
	assert.Equal(t, " nil.\n\n> _Ugh", r.Write(` nil.","remark":"Ugh`))
	assert.Equal(t, "._\n\n", r.Write(`."}`+"\n"))

	assert.Equal(t, "## Verdict\n\nNo", r.Write(`{"type":"verdict","text":"No`))
	assert.Equal(t, ".\n\n", r.Write(`."}`), "a complete object renders in full before its newline")
	assert.Empty(t, r.Close())
}

// A suggestion's code block appears whole once the suggestion is complete;
// the prose around it streams.
func TestRendererShowsSuggestionWhenComplete(t *testing.T) {
	t.Parallel()

	var r Renderer
	assert.Equal(t, "### [LOW] T\n\n`a.go:1`\n\nB", r.Write(`{"file":"a.go","line":1,"severity":"low","title":"T","body":"B`))
	assert.Equal(t, "\n\n", r.Write(`","suggestion":"x := `))
	assert.Equal(t, "```go\nx := 1\n```\n\n> _R", r.Write(`1","remark":"R`))
}

// Plain text from a model that ignores the format streams as it arrives.
func TestRendererStreamsFreeText(t *testing.T) {
	t.Parallel()

	var r Renderer
	assert.Equal(t, "## Rev", r.Write("## Rev"))
	assert.Equal(t, "iew\nLoo", r.Write("iew\nLoo"))
	assert.Equal(t, "ks fine.\n", r.Write("ks fine.")+r.Close())
}

// When fields arrive in a different order from the one they render in, the
// preview stops instead of guessing, and the finished finding is written in
// full below whatever was shown.
func TestRendererFieldsOutOfOrder(t *testing.T) {
	t.Parallel()

	output := `{"severity":"high","title":"T","body":"Body text","file":"a.go","line":3}` + "\n"
	final := Markdown(Parse(output))

	for size := 1; size <= len(output); size++ {
		got := renderInChunks(output, size)
		assert.True(t, strings.HasSuffix(got, final), "chunk size %d: %q", size, got)
	}
}

func TestParsePartial(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantText string
		wantOpen string
		wantLine int
	}{
		{name: "open string", input: `{"type":"intro","text":"ab`, wantText: "ab", wantOpen: "text"},
		{name: "cut escape", input: `{"type":"intro","text":"ab\`, wantText: "ab", wantOpen: "text"},
		{name: "cut unicode escape", input: `{"type":"intro","text":"ab\u00`, wantText: "ab", wantOpen: "text"},
		{name: "complete escapes", input: `{"type":"intro","text":"a\"bé\\`, wantText: `a"bé\`, wantOpen: "text"},
		{name: "cut multi-byte character", input: "{\"type\":\"intro\",\"text\":\"caf\xc3", wantText: "caf", wantOpen: "text"},
		{name: "number still arriving is left out", input: `{"line":4`, wantLine: 0},
		{name: "complete number", input: `{"line":42,"ti`, wantLine: 42},
		{name: "dangling key", input: `{"line":42,"title":`, wantLine: 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l, open, ok := parsePartial(tt.input)
			require.True(t, ok)
			assert.Equal(t, tt.wantText, l.Text)
			assert.Equal(t, tt.wantOpen, open)
			assert.Equal(t, tt.wantLine, l.Line)
		})
	}
}

func TestParsePartialRejectsNesting(t *testing.T) {
	t.Parallel()

	_, _, ok := parsePartial(`{"file":"a.go","extra":{"x":`)
	assert.False(t, ok)
}
