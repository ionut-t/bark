package tui

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ionut-t/bark/pkg/reviewers"
	"github.com/ionut-t/coffee/styles"
)

type listReviewersMsg struct{}

type reviewerSelectedMsg struct {
	Reviewer *reviewers.Reviewer
}

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

func newReviewersModel(reviewers []reviewers.Reviewer) reviewersModel {
	items := processReviewers(reviewers)

	l := list.New(items, itemDelegate{}, 80, 20)
	l.Title = "Select a Reviewer"

	l.Styles = styles.ListStyles()
	l.Styles.Title = l.Styles.Title.MarginLeft(2)

	l.FilterInput.PromptStyle = styles.Accent
	l.FilterInput.Cursor.Style = styles.Accent

	l.InfiniteScrolling = true
	l.SetShowStatusBar(false)

	l.KeyMap = listKeyMap()

	l.AdditionalShortHelpKeys = additionalHelpKeysFunc()
	l.AdditionalFullHelpKeys = additionalHelpKeysFunc()

	l.SetFilteringEnabled(true)

	return reviewersModel{
		list:      l,
		reviewers: reviewers,
	}
}

func (m reviewersModel) setSize(width, height int) {
	m.list.SetSize(width, height-4)
}

func (m reviewersModel) Init() tea.Cmd {
	return nil
}

func (m reviewersModel) Update(msg tea.Msg) (reviewersModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
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
			return m, tea.Quit
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m reviewersModel) View() string {
	return lipgloss.NewStyle().MarginTop(1).Render(m.list.View())
}
