package tui

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/utils"
	"github.com/ionut-t/coffee/styles"
)

type prDescriptionOptionSelectedMsg struct {
	useGitHubPR bool
}

type cancelPRDescriptionOptionsMsg struct{}

type prDescriptionOptionsModel struct {
	list list.Model
}

type prDescriptionOptionItem struct {
	title       string
	useGitHubPR bool
}

func (i prDescriptionOptionItem) Title() string       { return i.title }
func (i prDescriptionOptionItem) Description() string { return "" }
func (i prDescriptionOptionItem) FilterValue() string { return i.title }

var prDescriptionOptionsItems = []list.Item{
	prDescriptionOptionItem{title: "Current branch", useGitHubPR: false},
	prDescriptionOptionItem{title: "GitHub pull request", useGitHubPR: true},
}

func newPRDescriptionOptionsModel(s styles.Styles, isDarkMode bool) prDescriptionOptionsModel {
	l := newListModel("Generate PR description from", prDescriptionOptionsItems, s, isDarkMode)
	l.SetFilteringEnabled(false)

	return prDescriptionOptionsModel{list: l}
}

func (m prDescriptionOptionsModel) Init() tea.Cmd {
	return nil
}

func (m prDescriptionOptionsModel) Update(msg tea.Msg) (prDescriptionOptionsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.list.ResetSelected()
			return m, utils.DispatchMsg(cancelPRDescriptionOptionsMsg{})

		case "enter":
			i, ok := m.list.SelectedItem().(prDescriptionOptionItem)
			if !ok {
				return m, nil
			}
			return m, utils.DispatchMsg(prDescriptionOptionSelectedMsg{useGitHubPR: i.useGitHubPR})
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	return m, cmd
}

func (m prDescriptionOptionsModel) View() string {
	return renderList(m.list.View())
}
