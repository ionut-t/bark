package tui

import (
	"testing"

	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/ionut-t/coffee/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewModelSetSize(t *testing.T) {
	m := newReviewModel(reviewers.Reviewer{Name: "Spock"}, "s", "p", 80, 24, nil)
	m.setSize(100, 30)
	assert.Equal(t, 100, m.width)
	assert.Equal(t, 30, m.height)
}

func TestReviewHelp(t *testing.T) {
	s := styles.New(true)

	all := reviewHelp(120, false, s)
	assert.Contains(t, all, "Help")
	assert.Contains(t, all, "toggle between review and prompt")
	assert.Contains(t, all, "generate commit message (for staged changes)")
	assert.Contains(t, all, "show LLM usage stats")

	commits := reviewHelp(120, true, s)
	assert.Contains(t, commits, "toggle between review and prompt")
	assert.NotContains(t, commits, "generate commit message", "reviewing a commit can't make a commit message")
	assert.Contains(t, commits, "show LLM usage stats")
}

func TestReviewModelCanGenerateCommitMessage(t *testing.T) {
	m := newReviewModel(reviewers.Reviewer{Name: "Spock"}, "s", "p", 80, 24, nil)
	assert.True(t, m.canGenerateCommitMessage())

	m.editor, _ = m.editor.Update(keyPress('/'))
	require.True(t, m.editor.IsSearchMode())
	assert.False(t, m.canGenerateCommitMessage(), "not while searching")
}

func TestReviewModelInit(t *testing.T) {
	assert.Nil(t, newReviewModel(reviewers.Reviewer{Name: "Spock"}, "s", "p", 80, 24, nil).Init())
}
