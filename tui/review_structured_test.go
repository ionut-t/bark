package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/check"
	"github.com/ionut-t/bark/v2/internal/debuglog"
	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStructuredReview(t *testing.T) reviewModel {
	t.Helper()

	m := newReviewModel(reviewers.Reviewer{Name: "Spock"}, "system", "prompt", 80, 40, nil)
	m.setStructured(true)
	// The returned command would start the stream; the test sends the
	// stream messages itself.
	_ = m.startReview(context.Background())
	return m
}

// Findings stream into the editor as they arrive; the loading screen stays
// up until there is something to show.
func TestReviewStructuredRendersCompleteLines(t *testing.T) {
	t.Parallel()

	m := newStructuredReview(t)

	m, _ = m.Update(streamChunkMsg{content: `{"file":"a.go","line":2,"severity":"high",`})
	assert.True(t, m.loading, "nothing to render yet")

	m, _ = m.Update(streamChunkMsg{content: `"title":"Nil m`})
	require.False(t, m.loading)
	assert.Contains(t, m.editor.GetCurrentContent(), "### [HIGH] Nil m", "the title streams before its line completes")

	m, _ = m.Update(streamChunkMsg{content: `ap"}` + "\n" + `{"type":"verdict",`})
	content := m.editor.GetCurrentContent()
	assert.Contains(t, content, "### [HIGH] Nil map")
	assert.Contains(t, content, "`a.go:2`")
	assert.NotContains(t, content, `"type"`, "raw JSON must never be shown")

	m, _ = m.Update(streamChunkMsg{content: `"text":"Fix it."}`})
	m, _ = m.Update(streamCompleteMsg{})
	assert.Contains(t, m.response, "### [HIGH] Nil map")
	assert.Contains(t, m.response, "## Verdict\n\nFix it.", "the last line, without a newline, is flushed on completion")
}

// A retry starts from a fresh renderer, so a line cut off by an error isn't
// glued onto the new stream.
func TestReviewStructuredRetryResetsParser(t *testing.T) {
	t.Parallel()

	m := newStructuredReview(t)
	m, _ = m.Update(streamChunkMsg{content: `{"file":"a.go","title":"Lost`})
	m, _ = m.Update(streamErrorMsg{error: assert.AnError})

	_ = m.startReview(context.Background())
	m, _ = m.Update(streamChunkMsg{content: `{"type":"verdict","text":"Clean."}` + "\n"})
	m, _ = m.Update(streamCompleteMsg{})

	assert.Equal(t, "## Verdict\n\nClean.\n\n", m.contentBuilder.String())
}

// With BARK_DEBUG set, the raw output (not the rendered Markdown) and the
// stats go to a log file under the storage directory.
func TestReviewWritesDebugLog(t *testing.T) {
	t.Setenv(debuglog.EnvVar, "1")
	dir := t.TempDir()

	m := newReviewModel(reviewers.Reviewer{Name: "Spock"}, "SYSTEM", "PROMPT", 80, 40, nil)
	m.setStructured(true)
	m.setLogInfo(dir, "gemini")
	_ = m.startReview(context.Background())

	m, _ = m.Update(streamChunkMsg{content: `{"type":"verdict","text":"Fine."}`})
	_, _ = m.Update(streamCompleteMsg{usage: llm.Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3}, hasUsage: true})

	logs, err := filepath.Glob(filepath.Join(dir, "logs", "*-review.log"))
	require.NoError(t, err)
	require.Len(t, logs, 1)
	content, err := os.ReadFile(logs[0])
	require.NoError(t, err)

	assert.Contains(t, string(content), "provider:   gemini\n")
	assert.Contains(t, string(content), "reviewer:   Spock\nstructured: true\n", "the widest key, structured, sets the alignment")
	assert.Contains(t, string(content), "===== response (raw) =====\n"+`{"type":"verdict","text":"Fine."}`)
	assert.Contains(t, string(content), "tokens:       1 input, 2 output, 3 total\n")
}

// checkDiff is a diff a kept finding's evidence can quote.
const checkDiff = "+counts := make(map[string]int)"

// checkLLM answers the check's Generate call with output.
type checkLLM struct{ output string }

func (c checkLLM) Stream(context.Context, string, string) (<-chan llm.Response, <-chan error) {
	panic("the check doesn't stream")
}

func (c checkLLM) Generate(context.Context, string, string) (llm.Response, error) {
	return llm.Response{Content: c.output}, nil
}

// Once a structured review is done, its findings are checked and the
// doubted ones are marked where they are, so nothing the reader has seen
// moves and the verdict stays last.
func TestReviewStructuredRunsCheck(t *testing.T) {
	t.Parallel()

	m := newStructuredReview(t)
	m.setCheck(check.Review{Diff: checkDiff, Client: checkLLM{output: `{"id":1,"verdict":"drop","reason":"m is never nil."}` + "\n" +
		`{"id":2,"verdict":"keep","evidence":"counts := make(map[string]int)"}`}})

	m, _ = m.Update(streamChunkMsg{content: `{"file":"a.go","line":2,"severity":"high","title":"Nil map","body":"b"}` + "\n"})
	m, _ = m.Update(streamChunkMsg{content: `{"file":"b.go","line":5,"severity":"low","title":"Typo","body":"t"}` + "\n"})
	m, _ = m.Update(streamChunkMsg{content: `{"type":"verdict","text":"Fix it."}`})
	m, cmd := m.Update(streamCompleteMsg{})
	require.NotNil(t, cmd)
	assert.True(t, m.checking)

	msg := cmd()
	require.IsType(t, checkDoneMsg{}, msg)
	m, _ = m.Update(msg)

	assert.False(t, m.checking)
	assert.Equal(t, "### [HIGH] Nil map\n\n> **Doubted by Bark's check:** m is never nil.\n\n`a.go:2`\n\nb\n\n"+
		"### [LOW] Typo\n\n`b.go:5`\n\nt\n\n"+
		"## Verdict\n\nFix it.\n\n", m.response)
	assert.Equal(t, strings.TrimRight(m.response, "\n"), strings.TrimRight(m.editor.GetCurrentContent(), "\n"), "the checked review is shown, not only stored")
	assert.Equal(t, "Bark's check kept 1 of 2 findings", m.checkSummary)
}

// When the check keeps every finding, the review reads as it streamed.
func TestReviewStructuredCheckKeepsAll(t *testing.T) {
	t.Parallel()

	m := newStructuredReview(t)
	m.setCheck(check.Review{Diff: checkDiff, Client: checkLLM{output: `{"id":1,"verdict":"keep","evidence":"counts := make(map[string]int)"}`}})

	m, _ = m.Update(streamChunkMsg{content: `{"file":"a.go","line":2,"severity":"high","title":"Nil map","body":"b"}` + "\n"})
	m, _ = m.Update(streamChunkMsg{content: `{"type":"verdict","text":"Fix it."}`})
	m, cmd := m.Update(streamCompleteMsg{})
	streamed := m.response
	m, _ = m.Update(cmd())

	assert.Equal(t, strings.TrimRight(streamed, "\n"), strings.TrimRight(m.response, "\n"))
	assert.Equal(t, "Bark's check kept the finding", m.checkSummary)
}

// A failed check leaves the review as it streamed and says so at the end.
func TestReviewStructuredCheckFails(t *testing.T) {
	t.Parallel()

	m := newStructuredReview(t)
	m, _ = m.Update(streamChunkMsg{content: `{"file":"a.go","title":"T","body":"b"}` + "\n"})
	m.response = "### [MEDIUM] T\n\n`a.go`\n\nb\n\n"
	m.checking = true

	m, _ = m.Update(checkDoneMsg{run: m.run, err: assert.AnError})

	assert.Equal(t, "### [MEDIUM] T\n\n`a.go`\n\nb\n\n## Bark's check\n\n"+
		"The check failed, so the findings above are unchecked: "+assert.AnError.Error()+"\n", m.response)
}

// Marking findings after the check keeps the cursor on the line it was on,
// though the notes add lines above it.
func TestReviewCheckKeepsCursorOnFinding(t *testing.T) {
	t.Parallel()

	old := "### [HIGH] Nil map\n\n`a.go:2`\n\nb\n\n### [LOW] Typo\n\n`b.go:5`\n\nt\n"
	updated := "### [HIGH] Nil map\n\n> **Doubted by Bark's check:** m is never nil.\n\n`a.go:2`\n\nb\n\n### [LOW] Typo\n\n`b.go:5`\n\nt\n"

	assert.Equal(t, 10, followCursor(old, updated, 8), "on Typo's location line, below the note")
	assert.Equal(t, 6, followCursor(old, updated, 4), "on Nil map's body, below its own note")
	assert.Equal(t, 0, followCursor(old, updated, 0), "on the heading")
	assert.Equal(t, 1, followCursor("intro\n\n### [HIGH] Nil map\n", updated, 1), "above every heading, the cursor stays put")
	assert.Equal(t, 13, followCursor(old+"\n\n\n\n\n\n\n", updated, 20), "clamped to the new content")
}

// A check result for a review that was restarted is dropped.
func TestReviewStructuredIgnoresStaleCheck(t *testing.T) {
	t.Parallel()

	m := newStructuredReview(t)
	m.setCheck(check.Review{Client: checkLLM{}})
	m, _ = m.Update(streamChunkMsg{content: `{"file":"a.go","title":"T","body":"b"}` + "\n"})
	m, _ = m.Update(streamCompleteMsg{})
	require.True(t, m.checking)
	stale := checkDoneMsg{run: m.run, err: errors.New("stale")}

	_ = m.startReview(context.Background())
	m, _ = m.Update(streamChunkMsg{content: `{"type":"verdict","text":"Clean."}`})
	m, _ = m.Update(streamCompleteMsg{})
	m, _ = m.Update(stale)

	assert.NotContains(t, m.response, "stale")
}
