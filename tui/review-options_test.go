package tui

import (
	"testing"

	"github.com/ionut-t/coffee/styles"
	"github.com/stretchr/testify/assert"
)

func TestReviewOptionsModel(t *testing.T) {
	m := newReviewOptionsModel(styles.New(true), true)
	m.setStyles(styles.New(false), false)
	assert.Nil(t, m.Init())

	want := []ReviewOption{ReviewOptionCurrentChanges, ReviewOptionStagedChanges, ReviewPR, ReviewOptionCommit, ReviewOptionBranch}
	for i, option := range want {
		it := reviewOptionsItems[i].(reviewOptionItem)
		assert.Equal(t, option.String(), it.Title())
		assert.Equal(t, option.String(), it.FilterValue())
		assert.Empty(t, it.Description())

		_, cmd := m.Update(keyEnter)
		assert.Equal(t, reviewOptionSelectedMsg{option: option}, runCmd(t, cmd))
		m, _ = m.Update(keyDown)
	}

	m, cmd := m.Update(keyEsc)
	assert.Equal(t, cancelReviewOptionSelectionMsg{}, runCmd(t, cmd))
	assert.Zero(t, m.list.Index(), "esc resets the selection")

	assert.Contains(t, m.View(), "review option")
}
