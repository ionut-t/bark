package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
