package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/ionut-t/coffee/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommitsModel(t *testing.T) {
	commits := []git.Commit{
		{Hash: "abcdef1234567", Author: "Ann", Date: "2026-11-20", Message: "feat: x"},
		{Hash: "1234567abcdef", Author: "Bob", Date: "2026-11-19", Message: "fix: y"},
	}
	m := newCommitsModel(commits)
	m.setStyles(styles.New(true), true)
	m.setSize(100, 30)
	assert.Nil(t, m.Init())

	items := processCommits(commits)
	require.Len(t, items, 2)
	first := items[0].(commitItem)
	assert.Equal(t, "feat: x", first.Title())
	assert.Equal(t, "feat: x", first.FilterValue())
	assert.Equal(t, "abcdef1 by Ann (2026-11-20)", first.Description())

	_, cmd := m.Update(keyEnter)
	assert.Equal(t, commitSelectedMsg{commit: commits[0]}, runCmd(t, cmd))

	m, _ = m.Update(keyDown)
	_, cmd = m.Update(keyEnter)
	assert.Equal(t, commitSelectedMsg{commit: commits[1]}, runCmd(t, cmd))

	_, cmd = m.Update(keyEsc)
	assert.Equal(t, cancelCommitSelectionMsg{}, runCmd(t, cmd))

	m, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 10})
	assert.Equal(t, 60, m.list.Width())
	assert.Contains(t, m.View(), "Recent Commits")
}
