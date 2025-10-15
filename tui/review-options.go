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
	ReviewOptionCommit
)

func (r ReviewOption) String() string {
	switch r {
	case ReviewOptionCurrentChanges:
		return "Review current changes"
	case ReviewOptionCommit:
		return "Review a recent commit"
	default:
		return ""
	}
}

type reviewOptionSelectedMsg struct {
	option ReviewOption
}

type reviewOptionsModel struct {
	list list.Model
}

func newReviewOptionsModel() reviewOptionsModel {
	options := []list.Item{
		item{title: ReviewOptionCurrentChanges.String()},
		item{title: ReviewOptionCommit.String()},
	}

	l := newListModel("Select review option", options)
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
			return m, utils.DispatchMsg(changeViewMsg{view: viewTasks})

		case "enter":
			i, ok := m.list.SelectedItem().(item)
			if !ok {
				return m, nil
			}

			var option ReviewOption
			switch i.title {
			case ReviewOptionCurrentChanges.String():
				option = ReviewOptionCurrentChanges
			case ReviewOptionCommit.String():
				option = ReviewOptionCommit
			default:
				return m, nil
			}

			return m, utils.DispatchMsg(reviewOptionSelectedMsg{option: option})
		}
	}

	l, cmd := m.list.Update(msg)
	m.list = l

	return m, cmd
}

func (m reviewOptionsModel) View() string {
	return renderList(m.list.View())
}
