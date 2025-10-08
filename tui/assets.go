package tui

import (
	"os/exec"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ionut-t/bark/internal/config"
	"github.com/ionut-t/bark/pkg/instructions"
	"github.com/ionut-t/bark/pkg/reviewers"
	"github.com/ionut-t/coffee/styles"
)

type AssetType int

const (
	AssetInstruction AssetType = iota
	AssetReviewer
)

type AssetAction int

const (
	AssetActionDelete AssetAction = iota
	AssetActionEdit
)

type AssetsModel struct {
	storage   string
	list      list.Model
	assetType AssetType
	action    AssetAction
	error     error
	abort     bool
}

func NewAssetsModel(storagePath string, assetType AssetType, action AssetAction) AssetsModel {
	var items []list.Item

	switch assetType {
	case AssetInstruction:
		instructionsList, err := instructions.Get(storagePath)
		if err != nil {
			return AssetsModel{
				error: err,
			}
		}

		items = make([]list.Item, 0, len(instructionsList))

		for _, instruction := range instructionsList {
			items = append(items, item{
				title: instruction.Name,
			})
		}
	case AssetReviewer:
		reviewersList, err := reviewers.Get(storagePath)
		if err != nil {
			return AssetsModel{
				error: err,
			}
		}

		items = make([]list.Item, 0, len(reviewersList))

		for _, reviewer := range reviewersList {
			items = append(items, item{
				title: reviewer.Name,
			})
		}
	}

	m := AssetsModel{
		storage:   storagePath,
		list:      list.New(items, itemDelegate{}, 80, 24),
		assetType: assetType,
		action:    action,
	}

	m.list.Title = m.title()
	m.list.SetShowStatusBar(false)

	m.list.Styles = styles.ListStyles()
	m.list.Styles.Title = m.list.Styles.Title.MarginLeft(2)

	m.list.FilterInput.PromptStyle = styles.Accent
	m.list.FilterInput.Cursor.Style = styles.Accent

	m.list.InfiniteScrolling = true
	m.list.SetShowStatusBar(false)

	m.list.KeyMap = listKeyMap()

	m.list.AdditionalShortHelpKeys = additionalHelpKeysFunc()
	m.list.AdditionalFullHelpKeys = additionalHelpKeysFunc()

	m.list.SetFilteringEnabled(true)

	return m
}

func (m AssetsModel) Init() tea.Cmd {
	return nil
}

func (m AssetsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.QuitMsg:
		m.abort = true

	case tea.KeyMsg:
		if len(m.list.Items()) == 0 {
			m.abort = true
			return m, tea.Quit
		}

		switch msg.String() {
		case "ctrl+c", "esc":
			m.abort = true
			return m, tea.Quit

		case "enter":
			if len(m.list.Items()) == 0 {
				break
			}

			selectedItem := m.list.SelectedItem().(item)

			switch m.action {
			case AssetActionDelete:
				if err := m.delete(selectedItem.title); err != nil {
					m.error = err
				} else {
					m.abort = true
				}

				return m, tea.Quit
			case AssetActionEdit:
				path, err := m.edit(selectedItem.title)
				if err != nil {
					m.error = err
					return m, tea.Quit
				}

				return m, m.openInEditor(path)
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m AssetsModel) View() string {
	if m.abort {
		return ""
	}

	if m.error != nil {
		return styles.Error.Render(m.error.Error())
	}

	if len(m.list.Items()) == 0 {
		return styles.Warning.Padding(2).Render("No items found\n\nPress any key to exit")
	}

	return lipgloss.NewStyle().Padding(1, 2).Render(m.list.View())
}

func (m AssetsModel) title() string {
	var title string
	switch m.assetType {
	case AssetInstruction:
		title = "Select instruction"
	case AssetReviewer:
		title = "Select reviewer"
	}

	switch m.action {
	case AssetActionDelete:
		title += " to delete"
	case AssetActionEdit:
		title += " to edit"
	}

	return title
}

func (m AssetsModel) delete(name string) error {
	switch m.assetType {
	case AssetInstruction:
		return instructions.Delete(m.storage, name)
	case AssetReviewer:
		return reviewers.Delete(m.storage, name)
	}

	return nil
}

func (m AssetsModel) edit(name string) (string, error) {
	var path string
	var err error

	switch m.assetType {
	case AssetInstruction:
		path, err = instructions.GetPath(m.storage, name)
	case AssetReviewer:
		path, err = reviewers.GetPath(m.storage, name)
	}

	if err != nil {
		return "", err
	}

	return path, nil
}

func (m AssetsModel) openInEditor(path string) tea.Cmd {
	return tea.ExecProcess(exec.Command(config.GetEditor(), path), func(err error) tea.Msg {
		if err != nil {
			return tea.Quit
		}
		return nil
	})
}
