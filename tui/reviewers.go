package tui

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/internal/utils"
	"github.com/ionut-t/bark/pkg/reviewers"
	"github.com/ionut-t/coffee/styles"
)

type listReviewersMsg struct{}

type reviewerSelectedMsg struct {
	Reviewer *reviewers.Reviewer
}

type cancelReviewerSelectionMsg struct{}

func processReviewers(reviewers []reviewers.Reviewer) []list.Item {
	items := make([]list.Item, 0, len(reviewers))

	for _, reviewer := range reviewers {
		items = append(items, item{
			title:  reviewer.Name,
			prompt: reviewer.Prompt,
		})
	}

	return items
}

type reviewersModel struct {
	reviewers []reviewers.Reviewer
	list      list.Model
}

func newReviewersModel(reviewers []reviewers.Reviewer, s styles.Styles, isDarkMode bool) reviewersModel {
	return reviewersModel{
		reviewers: reviewers,
		list:      newListModel("Select reviewer", processReviewers(reviewers), s, isDarkMode),
	}
}

func (m reviewersModel) setSize(width, height int) {
	m.list.SetSize(width, height-4)
}

func (m reviewersModel) Init() tea.Cmd {
	return nil
}

func (m reviewersModel) Update(msg tea.Msg) (reviewersModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch msg.String() {
		case "esc":
			return m, utils.DispatchMsg(cancelReviewerSelectionMsg{})
		case "enter":
			if item, ok := m.list.SelectedItem().(item); ok {
				for _, reviewer := range m.reviewers {
					if reviewer.Name == item.title {
						return m, func() tea.Msg {
							return reviewerSelectedMsg{Reviewer: &reviewer}
						}
					}
				}
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	return m, cmd
}

func (m reviewersModel) View() string {
	return renderList(m.list.View())
}
