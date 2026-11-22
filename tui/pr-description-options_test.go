package tui

import (
	"testing"

	"github.com/ionut-t/coffee/styles"
	"github.com/stretchr/testify/assert"
)

func TestPRDescriptionOptionsModel(t *testing.T) {
	m := newPRDescriptionOptionsModel(styles.New(true), true)
	assert.Nil(t, m.Init())

	it := prDescriptionOptionsItems[1].(prDescriptionOptionItem)
	assert.Equal(t, "GitHub pull request", it.Title())
	assert.Equal(t, "GitHub pull request", it.FilterValue())
	assert.Empty(t, it.Description())

	_, cmd := m.Update(keyEnter)
	assert.Equal(t, prDescriptionOptionSelectedMsg{useGitHubPR: false}, runCmd(t, cmd))

	m, _ = m.Update(keyDown)
	_, cmd = m.Update(keyEnter)
	assert.Equal(t, prDescriptionOptionSelectedMsg{useGitHubPR: true}, runCmd(t, cmd))

	m, cmd = m.Update(keyEsc)
	assert.Equal(t, cancelPRDescriptionOptionsMsg{}, runCmd(t, cmd))
	assert.Zero(t, m.list.Index())

	assert.Contains(t, m.View(), "Generate PR description from")
}
