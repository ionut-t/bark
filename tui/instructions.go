package tui

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/internal/utils"
	"github.com/ionut-t/bark/pkg/instructions"
	"github.com/ionut-t/coffee/styles"
)

type instructionSelectedMsg struct {
	instruction string
}

type cancelInstructionSelectionMsg struct{}

type instructionsModel struct {
	list list.Model
}

func newInstructionsModel(instructions []instructions.Instruction, s styles.Styles, isDarkMode bool) instructionsModel {
	ls := newListModel("Select instruction", processInstructions(instructions), s, isDarkMode)

	ls.KeyMap = listKeyMap()

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

	ls.AdditionalShortHelpKeys = additionalKeys
	ls.AdditionalFullHelpKeys = additionalKeys

	return instructionsModel{
		list: ls,
	}
}

func (m instructionsModel) setSize(width, height int) {
	m.list.SetSize(width, height-4)
}

func (m instructionsModel) Init() tea.Cmd {
	return nil
}

func (m instructionsModel) Update(msg tea.Msg) (instructionsModel, tea.Cmd) {
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
				return m, utils.DispatchMsg(instructionSelectedMsg{instruction: selectedItem.prompt})
			}
			return m, nil

		case "x":
			return m, utils.DispatchMsg(instructionSelectedMsg{instruction: ""})
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	return m, cmd
}

func (m instructionsModel) View() string {
	return renderList(m.list.View())
}

func processInstructions(instructions []instructions.Instruction) []list.Item {
	items := make([]list.Item, 0, len(instructions))

	for _, instruction := range instructions {
		items = append(items, item{
			title:  instruction.Name,
			prompt: instruction.Prompt,
		})
	}

	return items
}
