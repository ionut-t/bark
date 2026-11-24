package tui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/instructions"
	"github.com/ionut-t/bark/v2/internal/utils"
	"github.com/ionut-t/coffee/styles"
)

type instructionSelectedMsg struct {
	instruction string
}

type cancelInstructionSelectionMsg struct{}

const instructionsTitle = "Select instructions"

type instructionsModel struct {
	list list.Model
}

// instructionItem is an instruction in the picker. Marked items are
// combined when the selection is confirmed.
type instructionItem struct {
	name, prompt string
	marked       bool
}

func (i instructionItem) Title() string {
	if i.marked {
		return "✓ " + i.name
	}
	return "  " + i.name
}

func (i instructionItem) FilterValue() string { return i.name }

func newInstructionsModel(instructions []instructions.Instruction, s styles.Styles, isDarkMode bool) instructionsModel {
	ls := newListModel(instructionsTitle, processInstructions(instructions), s, isDarkMode)

	ls.KeyMap = listKeyMap()

	additionalKeys := func() []key.Binding {
		return []key.Binding{
			key.NewBinding(
				key.WithKeys("space"),
				key.WithHelp("space", "mark"),
			),
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

func (m *instructionsModel) setSize(width, height int) {
	sizeList(&m.list, width, height)
}

func (m instructionsModel) Init() tea.Cmd {
	return nil
}

func (m instructionsModel) Update(msg tea.Msg) (instructionsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch msg.String() {
		case "esc":
			return m, utils.DispatchMsg(cancelInstructionSelectionMsg{})

		case "space":
			return m, m.toggleMarked()

		case "enter":
			if marked := m.markedPrompts(); len(marked) > 0 {
				return m, utils.DispatchMsg(instructionSelectedMsg{instruction: instructions.Join(marked)})
			}
			if selectedItem, ok := m.list.SelectedItem().(instructionItem); ok {
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

// toggleMarked marks the highlighted instruction, or unmarks it, and shows
// how many are marked in the title. The command it returns refreshes a
// filtered list, which shows a copy of the items.
func (m *instructionsModel) toggleMarked() tea.Cmd {
	selected, ok := m.list.SelectedItem().(instructionItem)
	if !ok {
		return nil
	}
	selected.marked = !selected.marked
	cmd := m.list.SetItem(m.list.GlobalIndex(), selected)

	m.list.Title = instructionsTitle
	if n := len(m.markedPrompts()); n > 0 {
		m.list.Title = fmt.Sprintf("%s (%d marked)", instructionsTitle, n)
	}
	return cmd
}

// markedPrompts returns the prompts of the marked instructions in list
// order, including any hidden by a filter.
func (m instructionsModel) markedPrompts() []string {
	var prompts []string
	for _, listItem := range m.list.Items() {
		if i, ok := listItem.(instructionItem); ok && i.marked {
			prompts = append(prompts, i.prompt)
		}
	}
	return prompts
}

func (m instructionsModel) View() string {
	return renderList(m.list.View())
}

func processInstructions(instructions []instructions.Instruction) []list.Item {
	items := make([]list.Item, 0, len(instructions))

	for _, instruction := range instructions {
		items = append(items, instructionItem{
			name:   instruction.Name,
			prompt: instruction.Prompt,
		})
	}

	return items
}
