package tui

import (
	"testing"

	"github.com/ionut-t/coffee/styles"
	"github.com/stretchr/testify/assert"
)

func TestPRNumberInputModel(t *testing.T) {
	m := newPRNumberInputModel("42")
	m.setStyles(styles.New(true))
	assert.Nil(t, m.Init())

	_, cmd := m.Update(keyEnter)
	assert.Equal(t, prNumberSelectedMsg{prNumber: "42"}, runCmd(t, cmd))

	_, cmd = m.Update(keyEsc)
	assert.Equal(t, cancelPRNumberSelectionMsg{}, runCmd(t, cmd))

	assert.Contains(t, m.View(), "esc")

	empty := newPRNumberInputModel("")
	empty.setStyles(styles.New(true))
	empty, _ = empty.Update(keyEnter)
	assert.EqualError(t, empty.inputErr, "PR number cannot be empty")
	assert.Contains(t, empty.View(), "PR number cannot be empty")
}
