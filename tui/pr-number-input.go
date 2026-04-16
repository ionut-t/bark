package tui

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/ionut-t/bark/v2/internal/utils"
	"github.com/ionut-t/coffee/styles"
)

type prNumberSelectedMsg struct {
	prNumber string
}

type cancelPRNumberSelectionMsg struct{}

type prNumberInputModel struct {
	prNumberInput *huh.Input
	inputErr      error
	styles        styles.Styles
}

func newPRNumberInputModel(prNumber string) prNumberInputModel {
	input := huh.NewInput().Title("PR Number")

	input.Focus()
	input.Value(&prNumber)

	return prNumberInputModel{
		prNumberInput: input,
	}
}

func (m *prNumberInputModel) setStyles(s styles.Styles) {
	m.styles = s
	m.prNumberInput.WithTheme(styles.HuhThemeCatppuccin{Styles: s})
}

func (m prNumberInputModel) Init() tea.Cmd {
	return nil
}

func (m prNumberInputModel) Update(msg tea.Msg) (prNumberInputModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, utils.DispatchMsg(cancelPRNumberSelectionMsg{})

		case "enter":
			if prNumber := m.prNumberInput.GetValue(); prNumber != "" {
				m.inputErr = nil

				return m, utils.DispatchMsg(prNumberSelectedMsg{
					prNumber: prNumber.(string),
				})
			}
		}
	}

	prNumberInput, cmd := m.prNumberInput.Update(msg)
	m.prNumberInput = prNumberInput.(*huh.Input)

	if m.prNumberInput.GetValue() == "" {
		m.inputErr = errors.New("PR number cannot be empty")
	} else {
		m.inputErr = nil
	}

	return m, cmd
}

func (m prNumberInputModel) View() string {
	var footer string

	if m.inputErr != nil {
		footer = m.styles.Error.Render(m.inputErr.Error())
	} else {
		footer = m.renderHelp()
	}

	return m.inputView(footer)
}

func (m *prNumberInputModel) renderHelp() string {
	key := m.styles.Subtext0.Render
	desc := m.styles.Overlay1.Render

	help := key("enter") + desc(" select")
	help += desc(" • ") + key("esc") + desc(" back")
	help += desc(" • ") + key("ctrl+c") + desc(" quit")

	return help
}

func (m prNumberInputModel) inputView(footer string) string {
	return lipgloss.NewStyle().Margin(2).Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			m.prNumberInput.View(),
			"\n\n",
			footer,
		),
	)
}
