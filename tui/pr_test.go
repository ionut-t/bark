package tui

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
)

func TestPRModelSetSize(t *testing.T) {
	m := newPRModel(nil, 80, 24)
	m.setSize(50, 10)
	assert.Equal(t, 10, lipgloss.Height(m.editor.View()))
}

func TestPRModelInit(t *testing.T) {
	assert.Nil(t, newPRModel(nil, 80, 24).Init())
}
