package tui

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/internal/utils"
	"github.com/ionut-t/coffee/styles"
)

type ReviewOption int

const (
	ReviewOptionNone ReviewOption = iota
	ReviewOptionCurrentChanges
	ReviewOptionStagedChanges
	ReviewOptionCommit
	ReviewOptionBranch
)

func (r ReviewOption) String() string {
	switch r {
	case ReviewOptionCurrentChanges:
		return "Review current changes"
	case ReviewOptionStagedChanges:
		return "Review staged changes"
	case ReviewOptionCommit:
		return "Review a recent commit"
	case ReviewOptionBranch:
		return "Review against a branch"
	default:
		return ""
	}
}

type reviewOptionSelectedMsg struct {
	option ReviewOption
}

type cancelReviewOptionSelectionMsg struct{}

type reviewOptionsModel struct {
	list list.Model
}

type reviewOptionItem struct {
	id    ReviewOption
	title string
}

func (i reviewOptionItem) Title() string       { return i.title }
func (i reviewOptionItem) Description() string { return "" }
func (i reviewOptionItem) FilterValue() string { return i.title }

var reviewOptionsItems = []list.Item{
	reviewOptionItem{id: ReviewOptionCurrentChanges, title: ReviewOptionCurrentChanges.String()},
	reviewOptionItem{id: ReviewOptionStagedChanges, title: ReviewOptionStagedChanges.String()},
	reviewOptionItem{id: ReviewOptionCommit, title: ReviewOptionCommit.String()},
	reviewOptionItem{id: ReviewOptionBranch, title: ReviewOptionBranch.String()},
}

func newReviewOptionsModel(s styles.Styles, isDarkMode bool) reviewOptionsModel {
	l := newListModel("Select review option", reviewOptionsItems, s, isDarkMode)
	l.SetFilteringEnabled(false)

	return reviewOptionsModel{
		list: l,
	}
}

func (m *reviewOptionsModel) setStyles(s styles.Styles, isDarkMode bool) {
	m.list = newListModel("Select a review option", reviewOptionsItems, s, isDarkMode)
	m.list.SetFilteringEnabled(false)
}

func (m reviewOptionsModel) Init() tea.Cmd {
	return nil
}

func (m reviewOptionsModel) Update(msg tea.Msg) (reviewOptionsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.list.ResetSelected()

			return m, utils.DispatchMsg(cancelReviewOptionSelectionMsg{})

		case "enter":
			i, ok := m.list.SelectedItem().(reviewOptionItem)
			if !ok {
				return m, nil
			}

			return m, utils.DispatchMsg(reviewOptionSelectedMsg{option: i.id})
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	return m, cmd
}

func (m reviewOptionsModel) View() string {
	return renderList(m.list.View())
}
