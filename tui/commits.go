package tui

import (
	"fmt"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/internal/utils"
	"github.com/ionut-t/bark/pkg/git"
	"github.com/ionut-t/coffee/styles"
)

type listCommitsMsg struct{}

type commitSelectedMsg struct {
	commit git.Commit
}

type cancelCommitSelectionMsg struct{}

type commitsModel struct {
	list list.Model
}

func newCommitsModel(commits []git.Commit) commitsModel {
	l := list.New(processCommits(commits), list.NewDefaultDelegate(), 80, 20)
	l.Title = "Recent Commits"
	l.SetShowStatusBar(false)

	l.InfiniteScrolling = true
	l.SetShowStatusBar(false)

	return commitsModel{
		list: l,
	}
}

func (m *commitsModel) setStyles(s styles.Styles, isDark bool) {
	m.list.Styles = styles.ListStyles(s, isDark)

	delegate := list.NewDefaultDelegate()
	delegate.Styles = styles.ListItemStyles(s, isDark)
	m.list.SetDelegate(delegate)
}

func (m *commitsModel) setSize(width, height int) {
	m.list.SetSize(width, height-4)
}

func (m commitsModel) Init() tea.Cmd {
	return nil
}

func (m commitsModel) Update(msg tea.Msg) (commitsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch msg.String() {
		case "esc":
			return m, utils.DispatchMsg(cancelCommitSelectionMsg{})

		case "enter":
			i, ok := m.list.SelectedItem().(commitItem)
			if ok {
				return m, utils.DispatchMsg(commitSelectedMsg{commit: git.Commit(i)})
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	return m, cmd
}

func (m commitsModel) View() string {
	return renderList(m.list.View())
}

type commitItem git.Commit

func (i commitItem) Title() string { return i.Message }
func (i commitItem) Description() string {
	return fmt.Sprintf("%s by %s (%s)", i.Hash[:7], i.Author, i.Date)
}
func (i commitItem) FilterValue() string { return i.Message }

func processCommits(commits []git.Commit) []list.Item {
	items := make([]list.Item, 0, len(commits))

	for _, commit := range commits {
		items = append(items, commitItem(commit))
	}
	return items
}
