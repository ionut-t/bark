package tui

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ionut-t/bark/internal/utils"
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

func newReviewOptionsModel() reviewOptionsModel {
	l := newListModel("Select review option", reviewOptionsItems)
	l.SetFilteringEnabled(false)

	return reviewOptionsModel{
		list: l,
	}
}

func (m reviewOptionsModel) Init() tea.Cmd {
	return nil
}

func (m reviewOptionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

	l, cmd := m.list.Update(msg)
	m.list = l

	return m, cmd
}

func (m reviewOptionsModel) View() string {
	return renderList(m.list.View())
}
