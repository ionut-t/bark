package tui

import (
	"log"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ionut-t/bark/v2/internal/instructions"
	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/ionut-t/bark/v2/internal/utils"
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
	editor    string
	list      list.Model
	assetType AssetType
	action    AssetAction
	error     error
	abort     bool
	styles    styles.Styles
}

func NewAssetsModel(storagePath string, assetType AssetType, action AssetAction, editor string) AssetsModel {
	// Dark until the terminal answers Init's request: see Model.Init.
	isDarkMode := true
	appStyles := styles.New(isDarkMode)

	// A load error leaves the list empty rather than unbuilt: Update still
	// passes it messages, and a zero list.Model panics on them.
	var (
		items   []list.Item
		loadErr error
	)

	switch assetType {
	case AssetInstruction:
		instructionsList, err := instructions.Get(storagePath)
		if err != nil {
			loadErr = err
			break
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
			loadErr = err
			break
		}

		items = make([]list.Item, 0, len(reviewersList))

		for _, reviewer := range reviewersList {
			items = append(items, item{
				title: reviewer.Name,
			})
		}
	}

	ls := newListModel(getTitle(assetType, action), items, appStyles, isDarkMode)
	ls.Styles = styles.ListStyles(appStyles, isDarkMode)

	return AssetsModel{
		storage:   storagePath,
		editor:    editor,
		assetType: assetType,
		action:    action,
		list:      ls,
		styles:    appStyles,
		error:     loadErr,
	}
}

func (m AssetsModel) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

func (m AssetsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.styles = styles.New(msg.IsDark())
		restyleList(&m.list, m.styles, msg.IsDark())
		return m, nil

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

				cmd, err := m.openInEditor(path)
				if err != nil {
					m.error = err
					return m, nil
				}

				return m, cmd
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m AssetsModel) View() tea.View {
	view := tea.NewView(m.createView())
	view.AltScreen = true

	return view
}

func (m AssetsModel) createView() string {
	if m.abort {
		return ""
	}

	if m.error != nil {
		return m.styles.Error.Render(m.error.Error())
	}

	if len(m.list.Items()) == 0 {
		return m.styles.Warning.Padding(2).Render("No items found\n\nPress any key to exit")
	}

	return lipgloss.NewStyle().Padding(1, 2).Render(m.list.View())
}

func getTitle(assetType AssetType, action AssetAction) string {
	var title string
	switch assetType {
	case AssetInstruction:
		title = "Select instruction"
	case AssetReviewer:
		title = "Select reviewer"
	}

	switch action {
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

func (m AssetsModel) openInEditor(path string) (tea.Cmd, error) {
	cmd, err := utils.OpenInEditorCmd(m.editor, path)
	if err != nil {
		return nil, err
	}

	processedCmd := tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			log.Printf("Error running editor command: %v", err)
			return tea.Quit
		}
		return nil
	})

	return processedCmd, nil
}
