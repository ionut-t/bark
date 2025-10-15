package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ionut-t/coffee/styles"
)

var (
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(styles.Primary.GetForeground()).Bold(true)
	renderList        = lipgloss.NewStyle().MarginTop(1).Render
)

type item struct {
	title, prompt string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return "" }
func (i item) FilterValue() string { return i.title }

type itemDelegate struct{}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i := listItem.(item)

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	_, _ = fmt.Fprint(w, fn(i.title))
}

func listKeyMap() list.KeyMap {
	defaultKeyMap := list.DefaultKeyMap()

	return list.KeyMap{
		CursorUp:   defaultKeyMap.CursorUp,
		CursorDown: defaultKeyMap.CursorDown,
		Filter:     defaultKeyMap.Filter,
		NextPage:   defaultKeyMap.NextPage,
		PrevPage:   defaultKeyMap.PrevPage,
		GoToStart:  defaultKeyMap.GoToStart,
		GoToEnd:    defaultKeyMap.GoToEnd,
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),

		ShowFullHelp:         defaultKeyMap.ShowFullHelp,
		CloseFullHelp:        defaultKeyMap.CloseFullHelp,
		AcceptWhileFiltering: defaultKeyMap.AcceptWhileFiltering,
		CancelWhileFiltering: defaultKeyMap.CancelWhileFiltering,
	}
}

func additionalHelpKeysFunc() func() []key.Binding {
	return func() []key.Binding {
		return []key.Binding{
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "select"),
			),
		}
	}
}

func newListModel(title string, items []list.Item) list.Model {
	l := list.New(items, itemDelegate{}, 80, 20)
	l.Title = title

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

	return l
}
