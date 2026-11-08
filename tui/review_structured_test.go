package tui

import (
	"context"
	"testing"

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
