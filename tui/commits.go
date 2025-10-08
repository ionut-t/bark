package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ionut-t/bark/pkg/git"
	"github.com/ionut-t/coffee/styles"
)

type listCommitsMsg struct{}

type commitSelectedMsg struct {
	commit git.Commit
}

type commitsModel struct {
	list list.Model
}

func newCommitsModel(commits []git.Commit) commitsModel {
	delegate := list.NewDefaultDelegate()
	delegate.Styles = styles.ListItemStyles()

	l := list.New(processCommits(commits), delegate, 80, 20)
	l.Title = "Recent Commits"
	l.SetShowStatusBar(false)

	l.Styles = styles.ListStyles()

	l.FilterInput.PromptStyle = styles.Accent
	l.FilterInput.Cursor.Style = styles.Accent

	l.InfiniteScrolling = true
	l.SetShowStatusBar(false)

	return commitsModel{
		list: l,
	}
}

func (c *commitsModel) setSize(width, height int) {
	c.list.SetSize(width, height-4)
}

func (c commitsModel) Init() tea.Cmd {
	return nil
}

func (c commitsModel) Update(msg tea.Msg) (commitsModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.list.SetSize(msg.Width, msg.Height)
	case tea.KeyMsg:
		if c.list.FilterState() == list.Filtering {
			break
		}
		switch keypress := msg.String(); keypress {
		case "enter":
			i, ok := c.list.SelectedItem().(commitItem)
			if ok {
				return c, func() tea.Msg {
					return commitSelectedMsg{commit: git.Commit(i)}
				}
			}
		}
	}

	var cmd tea.Cmd
	c.list, cmd = c.list.Update(msg)
	cmds = append(cmds, cmd)

	return c, tea.Batch(cmds...)
}

func (c commitsModel) View() string {
	return c.list.View()
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
