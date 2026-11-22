package tui

import (
	"testing"

	"github.com/ionut-t/coffee/styles"
	"github.com/stretchr/testify/assert"
)

func TestBranchInputModel(t *testing.T) {
	m := newBranchInputModel("feat/x")
	m.setStyles(styles.New(true))
	assert.Nil(t, m.Init())

	_, cmd := m.Update(keyEnter)
	assert.Equal(t, branchSelectedMsg{branch: "feat/x"}, runCmd(t, cmd))

	_, cmd = m.Update(keyEsc)
	assert.Equal(t, cancelBranchSelectionMsg{}, runCmd(t, cmd))

	assert.Contains(t, m.View(), "esc")

	empty := newBranchInputModel("")
	empty.setStyles(styles.New(true))
	empty, cmd = empty.Update(keyEnter)
	if cmd != nil {
		assert.NotEqual(t, branchSelectedMsg{}, cmd(), "an empty branch is never selected")
	}
	assert.EqualError(t, empty.inputErr, "branch name cannot be empty")
	assert.Contains(t, empty.View(), "branch name cannot be empty")
}
