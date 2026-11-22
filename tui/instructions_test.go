package tui

import (
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/instructions"
	"github.com/ionut-t/coffee/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	keySpace = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	keyDown  = tea.KeyPressMsg{Code: tea.KeyDown}
	keyEnter = tea.KeyPressMsg{Code: tea.KeyEnter}
)

func newTestInstructionsModel() instructionsModel {
	return newInstructionsModel([]instructions.Instruction{
		{Name: "security", Prompt: "Focus on security.\n"},
		{Name: "performance", Prompt: "Focus on performance.\n"},
		{Name: "style", Prompt: "Focus on style.\n"},
	}, styles.New(true), true)
}

// press sends keys to m in order, returning the model and the last command.
func press(m instructionsModel, keys ...tea.KeyPressMsg) (instructionsModel, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		m, cmd = m.Update(k)
	}
	return m, cmd
}

func selectedInstruction(t *testing.T, cmd tea.Cmd) string {
	t.Helper()
	require.NotNil(t, cmd)
	msg, ok := cmd().(instructionSelectedMsg)
	require.True(t, ok, "expected instructionSelectedMsg")
	return msg.instruction
}

func TestInstructionsEnterWithoutMarksSelectsHighlighted(t *testing.T) {
	_, cmd := press(newTestInstructionsModel(), keyDown, keyEnter)
	assert.Equal(t, "Focus on performance.\n", selectedInstruction(t, cmd))
}

func TestInstructionsEnterCombinesMarkedInListOrder(t *testing.T) {
	// Mark style first, then security: the result follows the list.
	m, _ := press(newTestInstructionsModel(), keyDown, keyDown, keySpace, keyDown, keySpace)
	assert.Equal(t, "Select instructions (2 marked)", m.list.Title)

	_, cmd := press(m, keyEnter)
	assert.Equal(t, "Focus on security.\n\nFocus on style.", selectedInstruction(t, cmd))
}

func TestInstructionsSpaceUnmarks(t *testing.T) {
	m, _ := press(newTestInstructionsModel(), keySpace, keySpace)
	assert.Equal(t, "Select instructions", m.list.Title)

	_, cmd := press(m, keyDown, keyEnter)
	assert.Equal(t, "Focus on performance.\n", selectedInstruction(t, cmd))
}

func TestInstructionsMarkedItemsShowCheck(t *testing.T) {
	m, _ := press(newTestInstructionsModel(), keySpace)
	assert.Contains(t, m.View(), "✓ security")
	assert.NotContains(t, m.View(), "✓ performance")
}

// run feeds the filter results cmd produces back into m, as Bubble Tea
// would. Other messages, such as the filter input's cursor blinks, are
// dropped so the test doesn't loop on them.
func run(m instructionsModel, cmd tea.Cmd) instructionsModel {
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			m = run(m, c)
		}
	case list.FilterMatchesMsg:
		m, cmd = m.Update(msg)
		m = run(m, cmd)
	}
	return m
}

func TestInstructionsMarkWhileFiltered(t *testing.T) {
	m := newTestInstructionsModel()
	m, cmd := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = run(m, cmd)
	for _, r := range "perf" {
		m, cmd = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = run(m, cmd)
	}
	m, cmd = m.Update(keyEnter) // apply the filter
	m = run(m, cmd)

	m, cmd = m.Update(keySpace)
	m = run(m, cmd)
	assert.Contains(t, m.View(), "✓ performance")

	_, cmd = m.Update(keyEnter)
	assert.Equal(t, "Focus on performance.", selectedInstruction(t, cmd))
}

func TestInstructionsModelInit(t *testing.T) {
	assert.Nil(t, newTestInstructionsModel().Init())
}

func TestInstructionsModelSetSize(t *testing.T) {
	m := newTestInstructionsModel()

	m.setSize(40, 20)

	assert.Equal(t, 40, m.list.Width())
	assert.Equal(t, 16, m.list.Height())
}
