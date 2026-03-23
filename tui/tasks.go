package tui

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/internal/utils"
	"github.com/ionut-t/coffee/styles"
)

type Task int

const (
	TaskNone Task = iota
	TaskReview
	TaskCommit
	TaskPRDescription
)

func (c Task) String() string {
	switch c {
	case TaskReview:
		return "Review"
	case TaskCommit:
		return "Generate commit message"
	case TaskPRDescription:
		return "Generate PR description"
	default:
		return ""
	}
}

type taskSelectedMsg struct {
	task Task
}

type tasksModel struct {
	list list.Model
}

var tasks = []list.Item{
	item{title: TaskReview.String()},
	item{title: TaskCommit.String()},
	item{title: TaskPRDescription.String()},
}

func newTasksModel(s styles.Styles, isDarkMode bool) tasksModel {
	l := newListModel("Select a task", tasks, s, isDarkMode)
	l.SetFilteringEnabled(false)

	return tasksModel{
		list: l,
	}
}

func (m *tasksModel) setStyles(s styles.Styles, isDarkMode bool) {
	m.list = newListModel("Select a task", tasks, s, isDarkMode)
	m.list.SetFilteringEnabled(false)
}

func (m tasksModel) Init() tea.Cmd {
	return nil
}

func (m tasksModel) Update(msg tea.Msg) (tasksModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if item, ok := m.list.SelectedItem().(item); ok {
				var task Task
				switch item.title {
				case TaskReview.String():
					task = TaskReview
				case TaskCommit.String():
					task = TaskCommit
				case TaskPRDescription.String():
					task = TaskPRDescription
				}

				return m, utils.DispatchMsg(taskSelectedMsg{task: task})
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	return m, cmd
}

func (m tasksModel) View() string {
	return renderList(m.list.View())
}
