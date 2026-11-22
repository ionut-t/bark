package tui

import (
	"testing"

	"github.com/ionut-t/coffee/styles"
	"github.com/stretchr/testify/assert"
)

func TestTasksModel(t *testing.T) {
	m := newTasksModel(styles.New(true), true)
	m.setStyles(styles.New(false), false)
	assert.Nil(t, m.Init())

	for _, task := range []Task{TaskReview, TaskCommit, TaskPRDescription} {
		_, cmd := m.Update(keyEnter)
		assert.Equal(t, taskSelectedMsg{task: task}, runCmd(t, cmd))
		m, _ = m.Update(keyDown)
	}
	assert.Empty(t, TaskNone.String())
	assert.Contains(t, m.View(), "Select a task")
}
