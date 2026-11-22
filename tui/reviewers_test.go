package tui

import (
	"testing"

	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/ionut-t/coffee/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewersModel(t *testing.T) {
	list := []reviewers.Reviewer{{Name: "Linus", Prompt: "l"}, {Name: "Yoda", Prompt: "y"}}

	items := processReviewers(list)
	require.Len(t, items, 2)
	assert.Equal(t, item{title: "Yoda", prompt: "y"}, items[1])

	m := newReviewersModel(list, styles.New(true), true)
	assert.Nil(t, m.Init())

	_, cmd := m.Update(keyEnter)
	assert.Equal(t, reviewerSelectedMsg{Reviewer: &list[0]}, runCmd(t, cmd))

	m, _ = m.Update(keyDown)
	_, cmd = m.Update(keyEnter)
	assert.Equal(t, reviewerSelectedMsg{Reviewer: &list[1]}, runCmd(t, cmd))

	_, cmd = m.Update(keyEsc)
	assert.Equal(t, cancelReviewerSelectionMsg{}, runCmd(t, cmd))

	assert.Contains(t, m.View(), "Select reviewer")
}

func TestReviewersModelSetSize(t *testing.T) {
	m := newReviewersModel([]reviewers.Reviewer{{Name: "Linus", Prompt: "l"}}, styles.New(true), true)

	m.setSize(40, 20)

	assert.Equal(t, 40, m.list.Width())
	assert.Equal(t, 16, m.list.Height())
}
