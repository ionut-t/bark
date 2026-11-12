package tui

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ionut-t/coffee/styles"
)

const (
	defaultListWidth  = 80
	defaultListHeight = 25
)

var (
	itemStyle  = lipgloss.NewStyle().PaddingLeft(4)
	renderList = lipgloss.NewStyle().MarginTop(1).Render
)

// the list.Item interface provides only the FilterValue function
type genericItem interface {
	Title() string
}

type item struct {
	title, prompt, key string
}

func (i item) Title() string       { return i.title }
func (i item) FilterValue() string { return i.title }

type itemDelegate struct {
	selectedStyle lipgloss.Style
}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i := listItem.(genericItem)

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return d.selectedStyle.Render("> " + strings.Join(s, " "))
		}
	}

	_, _ = fmt.Fprint(w, fn(i.Title()))
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

func newItemDelegate(s styles.Styles) itemDelegate {
	return itemDelegate{
		selectedStyle: s.Primary.PaddingLeft(2).Bold(true),
	}
}

// restyleList applies s to a list built by newListModel, keeping its items,
// cursor and filter.
func restyleList(l *list.Model, s styles.Styles, isDark bool) {
	l.Styles = styles.ListStyles(s, isDark)
	l.Styles.Title = l.Styles.Title.MarginLeft(2)
	l.SetDelegate(newItemDelegate(s))
}

func newListModel(title string, items []list.Item, s styles.Styles, isDark bool) list.Model {
	l := list.New(items, newItemDelegate(s), defaultListWidth, defaultListHeight)
	l.Title = title

	l.Styles = styles.ListStyles(s, isDark)
	l.Styles.Title = l.Styles.Title.MarginLeft(2)

	l.InfiniteScrolling = true
	l.SetShowStatusBar(false)

	l.KeyMap = listKeyMap()

	l.AdditionalShortHelpKeys = additionalHelpKeysFunc()
	l.AdditionalFullHelpKeys = additionalHelpKeysFunc()

	l.SetFilteringEnabled(true)

	return l
}
