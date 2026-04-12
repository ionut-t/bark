package tui

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/ionut-t/bark/v2/internal/utils"
	"github.com/ionut-t/coffee/styles"
)

type branchSelectedMsg struct {
	branch string
}

type cancelBranchSelectionMsg struct{}

type branchInputModel struct {
	branchInput *huh.Input
	inputErr    error
	styles      styles.Styles
}

func newBranchInputModel(branch string) branchInputModel {
	branchInput := huh.NewInput().Title("Branch")

	branchInput.Focus()
	branchInput.Value(&branch)

	return branchInputModel{
		branchInput: branchInput,
	}
}

func (m *branchInputModel) setStyles(s styles.Styles) {
	m.styles = s
	m.branchInput.WithTheme(styles.HuhThemeCatppuccin{Styles: s})
}

func (m branchInputModel) Init() tea.Cmd {
	return nil
}

func (m branchInputModel) Update(msg tea.Msg) (branchInputModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, utils.DispatchMsg(cancelBranchSelectionMsg{})

		case "enter":
			if branch := m.branchInput.GetValue(); branch != "" {
				m.inputErr = nil

				return m, utils.DispatchMsg(branchSelectedMsg{
					branch: branch.(string),
				})
			}
		}
	}

	branchInput, cmd := m.branchInput.Update(msg)
	m.branchInput = branchInput.(*huh.Input)

	if m.branchInput.GetValue() == "" {
		m.inputErr = errors.New("branch name cannot be empty")
	} else {
		m.inputErr = nil
	}

	return m, cmd
}

func (m branchInputModel) View() string {
	var footer string

	if m.inputErr != nil {
		footer = m.styles.Error.Render(m.inputErr.Error())
	} else {
		footer = m.renderHelp()
	}

	return m.inputView(footer)
}

func (m *branchInputModel) renderHelp() string {
	key := m.styles.Subtext0.Render
	desc := m.styles.Overlay1.Render

	help := key("enter") + desc(" select")
	help += desc(" • ") + key("esc") + desc(" back")
	help += desc(" • ") + key("ctrl+c") + desc(" quit")

	return help
}

func (m branchInputModel) inputView(footer string) string {
	return lipgloss.NewStyle().Margin(2).Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			m.branchInput.View(),
			"\n\n",
			footer,
		),
	)
}
