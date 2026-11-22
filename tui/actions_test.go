package tui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/templates"
	"github.com/ionut-t/bark/v2/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var keyTab = tea.KeyPressMsg{Code: tea.KeyTab}

func newTestActionsModel(editor string) ActionsModel {
	m := NewActionsModel(config.New("", config.Data{Editor: editor}))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return next.(ActionsModel)
}

// sendActions sends msgs to m in order and returns the model and the last
// command.
func sendActions(m ActionsModel, msgs ...tea.Msg) (ActionsModel, tea.Cmd) {
	var cmd tea.Cmd
	for _, msg := range msgs {
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(ActionsModel)
	}
	return m, cmd
}

func TestWorkflowFileName(t *testing.T) {
	assert.Equal(t, "bark-review.yaml", reviewWorkflow.workflowFileName())
	assert.Equal(t, "bark-pr-description.yaml", prDescriptionWorkflow.workflowFileName())
	assert.Equal(t, "bark", actionsWorkflowOption(99).workflowFileName())
	assert.Empty(t, actionsWorkflowOption(99).string())
}

func TestActionsModelBasics(t *testing.T) {
	m := newTestActionsModel("vi")
	assert.NotNil(t, m.Init(), "asks for the background colour")
	assert.Equal(t, 120, m.width)
	assert.Equal(t, 40, m.height)

	v := m.View()
	assert.True(t, v.AltScreen)
	assert.Equal(t, "Bark Actions Setup", v.WindowTitle)
	assert.Contains(t, m.createView(), "Select workflows")
	assert.Contains(t, m.createView(), "select/deselect", "the first view's help lists the multiselect key")

	_, cmd := sendActions(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	assert.Equal(t, tea.Quit(), runCmd(t, cmd))
}

func TestActionsModelNothingSelectedStays(t *testing.T) {
	m, _ := sendActions(newTestActionsModel("vi"), keyEnter)
	assert.Equal(t, actionsWorkflowView, m.view)
}

func TestActionsModelSingleWorkflowFlow(t *testing.T) {
	m := newTestActionsModel("vi")

	m, _ = sendActions(m, keySpace, keyEnter)
	require.Equal(t, actionsWorkflowSummaryView, m.view, "one workflow skips the combined question")
	assert.Equal(t, []actionsWorkflowOption{reviewWorkflow}, m.selectedWorkflowOptions)
	assert.False(t, m.combinedWorkflow)
	assert.Equal(t, map[string]string{"bark-review.yaml": templates.GetDefaultReviewActionTemplate()}, m.summary.workflows)
	assert.True(t, m.summary.editorFocused, "a single workflow opens in the editor")
	assert.Contains(t, m.createView(), "Preview & Edit Workflows")
	assert.False(t, m.summary.shouldPreventExit())
	assert.Nil(t, m.summary.Init())

	m, _ = sendActions(m, keyEnter)
	require.Equal(t, actionsConfirmDirectoryView, m.view)
	assert.Contains(t, m.createView(), "Creates reviewer.md, review.md, and pr.md")

	m, _ = sendActions(m, keyEnter)
	require.Equal(t, actionsSaveView, m.view)
	view := m.createView()
	assert.Contains(t, view, "Confirm & Save")
	assert.Contains(t, view, "bark-review.yaml")
	assert.NotContains(t, view, "bark-pr-description.yaml")
	assert.Contains(t, view, ".bark/")

	// Back through every view.
	m, _ = sendActions(m, keyEsc)
	assert.Equal(t, actionsConfirmDirectoryView, m.view)
	m, _ = sendActions(m, keyEsc)
	assert.Equal(t, actionsWorkflowSummaryView, m.view)
	m, _ = sendActions(m, keyEsc)
	assert.Equal(t, actionsWorkflowView, m.view, "one workflow goes straight back to the selection")
}

func TestActionsModelCombinedWorkflowFlow(t *testing.T) {
	m := newTestActionsModel("vi")

	m, _ = sendActions(m, keySpace, keyDown, keySpace, keyEnter)
	require.Equal(t, actionsWorkflowStructureView, m.view)
	assert.ElementsMatch(t, []actionsWorkflowOption{reviewWorkflow, prDescriptionWorkflow}, m.selectedWorkflowOptions)
	assert.Contains(t, m.createView(), "combined workflow")

	m, _ = sendActions(m, keyEsc)
	assert.Equal(t, actionsWorkflowView, m.view)
	m, _ = sendActions(m, keyEnter)
	require.Equal(t, actionsWorkflowStructureView, m.view)

	m, _ = sendActions(m, keyEnter) // "Yes" is selected by default
	require.Equal(t, actionsWorkflowSummaryView, m.view)
	assert.True(t, m.combinedWorkflow)
	assert.Equal(t, map[string]string{"bark.yaml": templates.GetDefaultCombinedActionTemplate()}, m.summary.workflows)
	assert.Contains(t, m.createView(), "bark.yaml")

	m, _ = sendActions(m, keyEnter, keyEnter)
	require.Equal(t, actionsSaveView, m.view)
	assert.Contains(t, m.createView(), "bark.yaml")
	assert.NotContains(t, m.createView(), "bark-review.yaml")

	m, _ = sendActions(m, keyEsc, keyEsc, keyEsc)
	assert.Equal(t, actionsWorkflowStructureView, m.view, "several workflows go back to the combined question")
}

func TestActionsModelSeparateWorkflows(t *testing.T) {
	m := newTestActionsModel("vi")

	m, _ = sendActions(m, keySpace, keyDown, keySpace, keyEnter, keyDown, keyEnter)
	require.Equal(t, actionsWorkflowSummaryView, m.view)
	assert.False(t, m.combinedWorkflow)
	assert.Equal(t, map[string]string{
		"bark-review.yaml":         templates.GetDefaultReviewActionTemplate(),
		"bark-pr-description.yaml": templates.GetDefaultPRDescriptionActionTemplate(),
	}, m.summary.workflows)
	assert.False(t, m.summary.editorFocused, "the list has focus when there's a choice")
	assert.Contains(t, m.summary.renderHelp(), "switch to editor")

	m, _ = sendActions(m, keyTab)
	assert.True(t, m.summary.editorFocused)
	assert.Contains(t, m.summary.renderHelp(), "switch to list")
	m, _ = sendActions(m, keyTab)
	assert.False(t, m.summary.editorFocused)

	// Moving down the list shows the other workflow.
	m, _ = sendActions(m, keyDown)
	assert.Contains(t, m.summary.editor.GetCurrentContent(), "name: Bark PR Description")
	assert.Contains(t, m.summary.View(), ".github/workflows/bark-pr-description.yaml")
}

func TestActionsModelSaves(t *testing.T) {
	t.Chdir(t.TempDir())
	m := newTestActionsModel("vi")
	m, _ = sendActions(m, keySpace, keyEnter, keyEnter, keyDown, keyEnter)
	require.Equal(t, actionsSaveView, m.view)
	assert.NotContains(t, m.createView(), ".bark/", "no .bark/ when it was declined")

	m, cmd := sendActions(m, keyEnter)
	msg := runCmd(t, cmd)
	assert.Equal(t, actionsSavedMsg{}, msg)

	content, err := os.ReadFile(filepath.Join(".github", "workflows", "bark-review.yaml"))
	require.NoError(t, err)
	assert.Equal(t, templates.GetDefaultReviewActionTemplate(), string(content))
	_, err = os.Stat(".bark")
	assert.ErrorIs(t, err, os.ErrNotExist)

	_, cmd = sendActions(m, msg)
	assert.Equal(t, tea.Quit(), runCmd(t, cmd), "a successful save quits")
}

func TestActionsModelSaveError(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile(".github", []byte("not a dir"), 0o644))

	m := newTestActionsModel("vi")
	m, _ = sendActions(m, keySpace, keyEnter, keyEnter, keyEnter)
	_, cmd := sendActions(m, keyEnter)
	msg := runCmd(t, cmd)
	require.IsType(t, actionsSaveErrorMsg{}, msg)

	m, _ = sendActions(m, msg)
	require.Error(t, m.saveError)
	view := m.createView()
	assert.Contains(t, view, "retry")
	assert.NotContains(t, view, "Confirm & Save")

	m, _ = sendActions(m, keyEsc)
	assert.NoError(t, m.saveError, "going back clears the error")
	assert.Equal(t, actionsConfirmDirectoryView, m.view)
}

func TestActionsSummaryExternalEditor(t *testing.T) {
	// openInEditor writes the workflow to a temporary file first.
	t.Setenv("TMPDIR", t.TempDir())

	t.Run("editor not configured", func(t *testing.T) {
		m := newTestActionsModel("  ") // an empty editor gets a default
		m, _ = sendActions(m, keySpace, keyEnter)
		msg := runCmd(t, m.summary.openInEditor())
		require.IsType(t, actionsExternalEditorMsg{}, msg)
		assert.EqualError(t, msg.(actionsExternalEditorMsg).err, "editor is not configured")

		m, cmd := sendActions(m, msg)
		assert.NotNil(t, cmd, "the error is cleared later")
		assert.Contains(t, m.summary.renderHelp(), "Error: editor is not configured")

		m, _ = sendActions(m, utils.ClearMsg{})
		assert.NoError(t, m.summary.error)
	})

	t.Run("ctrl+e opens the editor", func(t *testing.T) {
		m := newTestActionsModel("vi")
		m, _ = sendActions(m, keySpace, keyEnter)
		_, cmd := sendActions(m, tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
		assert.NotNil(t, cmd)
	})

	t.Run("edited content replaces the workflow", func(t *testing.T) {
		m := newTestActionsModel("vi")
		m, _ = sendActions(m, keySpace, keyDown, keySpace, keyEnter, keyEnter)
		require.True(t, m.combinedWorkflow)

		m, _ = sendActions(m, actionsExternalEditorMsg{content: []byte("name: edited\n")})
		assert.Equal(t, "name: edited", m.summary.workflows["bark.yaml"], "the editor drops the trailing newline")
	})

	t.Run("a failed read is reported", func(t *testing.T) {
		m := newTestActionsModel("vi")
		m, _ = sendActions(m, keySpace, keyEnter)
		m, _ = sendActions(m, actionsExternalEditorMsg{err: errors.New("gone")})
		assert.EqualError(t, m.summary.error, "gone")
		assert.Equal(t, templates.GetDefaultReviewActionTemplate(), m.summary.workflows["bark-review.yaml"])
	})
}
