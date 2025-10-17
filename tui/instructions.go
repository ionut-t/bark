package tui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ionut-t/bark/internal/utils"
	"github.com/ionut-t/bark/pkg/instructions"
	"github.com/ionut-t/coffee/styles"
)

type instructionSelectedMsg struct {
	Instruction string
}

type cancelInstructionSelectionMsg struct{}

type instructionsModel struct {
	list    list.Model
	storage string
}

func newInstructionsModel(instructions []instructions.Instruction, storage string) instructionsModel {
	items := make([]list.Item, 0, len(instructions))

	for _, instruction := range instructions {
		items = append(items, item{
			title:  instruction.Name,
			prompt: instruction.Prompt,
		})
	}

	l := list.New(items, itemDelegate{}, 80, 24)
	l.Title = "Select instruction"
	l.SetShowStatusBar(false)

	l.Styles = styles.ListStyles()
	l.Styles.Title = l.Styles.Title.MarginLeft(2)

	l.FilterInput.PromptStyle = styles.Accent
	l.FilterInput.Cursor.Style = styles.Accent

	l.InfiniteScrolling = true
	l.SetShowStatusBar(false)

	l.KeyMap = listKeyMap()

	additionalKeys := func() []key.Binding {
		return []key.Binding{
			key.NewBinding(
				key.WithKeys("enter"),
				key.WithHelp("enter", "select"),
			),
			key.NewBinding(
				key.WithKeys("x"),
				key.WithHelp("x", "skip"),
			),
		}
	}

	l.AdditionalShortHelpKeys = additionalKeys
	l.AdditionalFullHelpKeys = additionalKeys

	l.SetFilteringEnabled(true)

	return instructionsModel{
		list:    l,
		storage: storage,
	}
}

func (m instructionsModel) setSize(width, height int) {
	m.list.SetSize(width, height-4)
}

func (m instructionsModel) Init() tea.Cmd {
	return nil
}

func (m instructionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height-4)
	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch msg.String() {
		case "esc":
			return m, utils.DispatchMsg(cancelInstructionSelectionMsg{})

		case "enter":
			if selectedItem, ok := m.list.SelectedItem().(item); ok {
				return m, utils.DispatchMsg(instructionSelectedMsg{Instruction: selectedItem.prompt})
			}
			return m, nil

		case "x":
			return m, utils.DispatchMsg(instructionSelectedMsg{Instruction: ""})
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m instructionsModel) View() string {
	return renderList(m.list.View())
}
