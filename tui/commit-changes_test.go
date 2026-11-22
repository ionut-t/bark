package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/coffee/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommitChangesModelSetSize(t *testing.T) {
	m := newCommitChangesModel(nil, "s", "p", false, 80, 24)
	m.setSize(100, 30)
	assert.Equal(t, 100, m.width)
	assert.Equal(t, 30, m.height)
	assert.Equal(t, 96, m.viewport.Width())
	assert.Equal(t, 25, m.viewport.Height())

	m.setSize(40, 6)
	assert.Equal(t, 5, m.viewport.Height(), "the viewport keeps 5 lines")
}

func TestCommitChangesModelHelpers(t *testing.T) {
	m := newCommitChangesModel(nil, "s", "p", true, 80, 24)
	m.setStyles(styles.New(true), true)

	assert.False(t, m.canRetry(), "not while loading (a new model starts loading)")
	m.loading = false
	assert.True(t, m.canRetry())
	m.editor.SetInsertMode()
	assert.False(t, m.canRetry(), "not while editing")

	m.error = errors.New("commit failed")
	got := m.getError()
	assert.Contains(t, got, "Error: commit failed")
	assert.Contains(t, got, "Press r to retry")
}

func TestCommitChangesModelDispatch(t *testing.T) {
	m := newCommitChangesModel(nil, "s", "p", true, 80, 24)
	m.editor.SetContent("feat: x")

	cmd := m.dispatch()
	require.NotNil(t, cmd)
	var found bool
	for _, c := range runCmd(t, cmd).(tea.BatchMsg) {
		if c == nil {
			continue
		}
		if msg, ok := c().(commitChangesMsg); ok {
			assert.Equal(t, commitChangesMsg{message: "feat: x", commitAll: true}, msg)
			found = true
		}
	}
	assert.True(t, found, "dispatch sends the message to commit")
}

func TestListenCommitOutput(t *testing.T) {
	t.Run("a line", func(t *testing.T) {
		out := make(chan string, 1)
		out <- "1 file changed"
		assert.Equal(t, commitOutputMsg{line: "1 file changed"}, listenCommitOutput(out, make(chan error))())
	})

	t.Run("output ends without an error", func(t *testing.T) {
		out, errs := make(chan string), make(chan error)
		close(out)
		close(errs)
		assert.Equal(t, commitStatusMsg{}, listenCommitOutput(out, errs)())
	})

	t.Run("output ends with an error", func(t *testing.T) {
		out, errs := make(chan string), make(chan error, 1)
		close(out)
		errs <- errors.New("nothing to commit")
		assert.Equal(t, commitStatusMsg{error: errors.New("nothing to commit")}, listenCommitOutput(out, errs)())
	})

	t.Run("an error before any output", func(t *testing.T) {
		errs := make(chan error, 1)
		errs <- errors.New("boom")
		assert.Equal(t, commitStatusMsg{error: errors.New("boom")}, listenCommitOutput(make(chan string), errs)())
	})

	t.Run("error channel closed first", func(t *testing.T) {
		errs := make(chan error)
		close(errs)
		assert.Equal(t, commitStatusMsg{}, listenCommitOutput(make(chan string), errs)())
	})
}

func TestCommitChangesModelInit(t *testing.T) {
	assert.Nil(t, newCommitChangesModel(nil, "s", "p", false, 80, 24).Init())
}
