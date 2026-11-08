package findings

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Lines models wrote on PR #104 that aren't valid JSON because of code
// copied into a string, shortened.
const (
	// Unescaped quotes in original: msg != "" {
	unescapedQuotes = `{"file":"internal/prreview/post.go","line":120,"severity":"high","category":"bug","title":"Stderr not captured","original":"\t\t\tif msg := strings.TrimSpace(string(exitErr.Stderr) + \"\\n\" + string(out)); msg != "" {","suggestion":"\t\t\tif msg := strings.TrimSpace(string(out)); msg != \"\" {","remark":"R."}`
	// An invalid escape in original: \.go
	invalidEscape = "{\"file\":\"internal/git/format.go\",\"line\":78,\"severity\":\"low\",\"category\":\"design\",\"title\":\"Fragile regex\",\"original\":\"\\tgoFileInDiff = regexp.MustCompile(`(?m)^diff --git a/.+ b/.+\\.go$`)\",\"remark\":\"R.\"}"
)

func TestParseRepairsCopiedCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		line string
		want Finding
	}{
		{
			name: "unescaped quotes",
			line: unescapedQuotes,
			want: Finding{
				File: "internal/prreview/post.go", Line: 120, Severity: High, Category: "bug", Title: "Stderr not captured",
				Original:   "\t\t\tif msg := strings.TrimSpace(string(exitErr.Stderr) + \"\\n\" + string(out)); msg != \"\" {",
				Suggestion: "\t\t\tif msg := strings.TrimSpace(string(out)); msg != \"\" {",
				Remark:     "R.",
			},
		},
		{
			name: "invalid escape",
			line: invalidEscape,
			want: Finding{
				File: "internal/git/format.go", Line: 78, Severity: Low, Category: "design", Title: "Fragile regex",
				Original: "\tgoFileInDiff = regexp.MustCompile(`(?m)^diff --git a/.+ b/.+\\.go$`)",
				Remark:   "R.",
			},
		},
		{
			name: "raw tab inside a string",
			line: "{\"file\":\"a.go\",\"line\":1,\"title\":\"T\",\"original\":\"\tx := 1\"}",
			want: Finding{File: "a.go", Line: 1, Severity: Medium, Title: "T", Original: "\tx := 1"},
		},
		{
			name: "quote before a comma that isn't a field",
			line: `{"file":"a.go","title":"T","original":"f("a", "b")","body":"B"}`,
			want: Finding{File: "a.go", Severity: Medium, Title: "T", Original: `f("a", "b")`, Body: "B"},
		},
		{
			name: "quote at the end of a value",
			line: `{"file":"a.go","title":"T","original":"s := """}`,
			want: Finding{File: "a.go", Severity: Medium, Title: "T", Original: `s := ""`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Error(t, json.Unmarshal([]byte(tt.line), &line{}), "the line must be invalid JSON")
			events := Parse(tt.line)
			require.Len(t, events, 1)
			require.NotNil(t, events[0].Finding, "got %+v", events[0])
			assert.Equal(t, tt.want, *events[0].Finding)
		})
	}
}

func TestParseRepairsIntroAndVerdict(t *testing.T) {
	t.Parallel()

	events := Parse(`{"type":"intro","text":"You call this "done"?"}` + "\n" + `{"type":"verdict","text":"Fix C:\data."}`)
	assert.Equal(t, []Event{{Intro: `You call this "done"?`}, {Verdict: `Fix C:\data.`}}, events)
}

// Repair only applies to lines that fail to parse; anything it can't fix
// stays text.
func TestParseLeavesUnrepairableLinesAsText(t *testing.T) {
	t.Parallel()

	for _, l := range []string{
		`{"file":"a.go","title":"T"`,
		`{"file":"a.go" "title":"T"}`,
		`{not json at all}`,
	} {
		assert.Equal(t, []Event{{Text: l}}, Parse(l), l)
	}
}

// A repaired line renders the same whether it's streamed or not.
func TestRendererRepairsStreamedLines(t *testing.T) {
	t.Parallel()

	output := unescapedQuotes + "\n" + invalidEscape + "\n" + `{"type":"verdict","text":"Done."}`
	want := Markdown(Parse(output))
	require.Contains(t, want, "### [HIGH · bug] Stderr not captured")
	require.Contains(t, want, "### [LOW · design] Fragile regex")

	for size := 1; size <= len(output); size++ {
		got := renderInChunks(output, size)
		require.Equal(t, want, got, "chunk size %d", size)
		require.False(t, strings.Contains(got, `{"file"`), "chunk size %d", size)
	}
}
