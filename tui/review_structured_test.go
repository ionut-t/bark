package tui

import (
	"context"
	"os"
	"path/filepath"
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

// checkLLM answers the check's Generate call with output.
type checkLLM struct{ output string }

func (c checkLLM) Stream(context.Context, string, string) (<-chan llm.Response, <-chan error) {
	panic("the check doesn't stream")
}

func (c checkLLM) Generate(context.Context, string, string) (llm.Response, error) {
	return llm.Response{Content: c.output}, nil
}

// Once a structured review is done, its findings are checked and the
// check's report is appended to the review.
func TestReviewStructuredRunsCheck(t *testing.T) {
	t.Parallel()

	m := newStructuredReview(t)
	m.setCheck(check.Review{Client: checkLLM{output: `{"id":1,"verdict":"drop","reason":"m is never nil."}`}})

	m, _ = m.Update(streamChunkMsg{content: `{"file":"a.go","line":2,"severity":"high","title":"Nil map","body":"b"}` + "\n"})
	m, _ = m.Update(streamChunkMsg{content: `{"type":"verdict","text":"Fix it."}`})
	m, cmd := m.Update(streamCompleteMsg{})
	require.NotNil(t, cmd)
	assert.True(t, m.checking)

	msg := cmd()
	require.IsType(t, checkDoneMsg{}, msg)
	m, _ = m.Update(msg)

	assert.False(t, m.checking)
	assert.Contains(t, m.response, "## Verdict\n\nFix it.\n\n## Bark's check\n\n"+
		"A second pass checked 1 finding against the code and doubts 1:\n\n"+
		"- **[HIGH] Nil map** `a.go:2`\n  m is never nil.\n")
	assert.Contains(t, m.editor.GetCurrentContent(), "m is never nil.", "the report is shown, not only stored")
}

// Without findings there is nothing to check.
func TestReviewStructuredNoFindingsNoCheck(t *testing.T) {
	t.Parallel()

	m := newStructuredReview(t)
	m.setCheck(check.Review{Client: checkLLM{}})

	m, _ = m.Update(streamChunkMsg{content: `{"type":"verdict","text":"Clean."}`})
	m, _ = m.Update(streamCompleteMsg{})

	assert.False(t, m.checking)
	assert.NotContains(t, m.response, "Bark's check")
}

// A check result for a review that was restarted is dropped.
func TestReviewStructuredIgnoresStaleCheck(t *testing.T) {
	t.Parallel()

	m := newStructuredReview(t)
	m.setCheck(check.Review{Client: checkLLM{}})
	m, _ = m.Update(streamChunkMsg{content: `{"file":"a.go","title":"T","body":"b"}` + "\n"})
	m, _ = m.Update(streamCompleteMsg{})
	require.True(t, m.checking)
	stale := checkDoneMsg{run: m.run, report: "## Bark's check\n\nstale\n"}

	_ = m.startReview(context.Background())
	m, _ = m.Update(streamChunkMsg{content: `{"type":"verdict","text":"Clean."}`})
	m, _ = m.Update(streamCompleteMsg{})
	m, _ = m.Update(stale)

	assert.NotContains(t, m.response, "stale")
}
