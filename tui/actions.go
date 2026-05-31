package tui

import (
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/scaffold"
	"github.com/ionut-t/bark/v2/internal/templates"
	"github.com/ionut-t/bark/v2/internal/utils"
	"github.com/ionut-t/coffee/styles"
	editor "github.com/ionut-t/goeditor"
)

const actionsListOuterWidth = 38

type actionsExternalEditorMsg struct {
	content []byte
	err     error
}

type (
	actionsSavedMsg     struct{}
	actionsSaveErrorMsg struct{ err error }
)

type actionsView int

const (
	actionsWorkflowView actionsView = iota
	actionsWorkflowStructureView
	actionsWorkflowSummaryView
	actionsConfirmDirectoryView
	actionsSaveView
)

type actionsWorkflowOption int

const (
	reviewWorkflow actionsWorkflowOption = iota
	prDescriptionWorkflow
)

func (o actionsWorkflowOption) string() string {
	switch o {
	case reviewWorkflow:
		return "Code review"
	case prDescriptionWorkflow:
		return "PR description"
	default:
		return ""
	}
}

func (o actionsWorkflowOption) workflowFileName() string {
	switch o {
	case reviewWorkflow:
		return "bark-review.yaml"
	case prDescriptionWorkflow:
		return "bark-pr-description.yaml"
	default:
		return "bark"
	}
}

type ActionsModel struct {
	width, height int

	config     config.Config
	styles     styles.Styles
	isDarkMode bool

	view actionsView

	selectedWorkflowOptions []actionsWorkflowOption
	combinedWorkflow        bool

	multiselectWorkflowInput      *huh.MultiSelect[actionsWorkflowOption]
	selectStructuredWorkflowInput *huh.Select[bool]
	confirmDirectoryInput         *huh.Select[bool]

	summary   actionsWorkflowSummaryModel
	saveError error
}

func NewActionsModel(cfg config.Config) ActionsModel {
	isDarkMode := styles.IsDark()
	appStyles := styles.New(isDarkMode)

	multiselectWorkflowInput := huh.NewMultiSelect[actionsWorkflowOption]().Title("Select workflows").
		Options(
			huh.NewOption(reviewWorkflow.string(), reviewWorkflow),
			huh.NewOption(prDescriptionWorkflow.string(), prDescriptionWorkflow),
		)

	multiselectWorkflowInput.WithKeyMap(huh.NewDefaultKeyMap())
	multiselectWorkflowInput.Focus()
	multiselectWorkflowInput.Height(3)
	multiselectWorkflowInput.WithTheme(styles.HuhThemeCatppuccin{Styles: appStyles})

	selectStructuredWorkflowInput := huh.NewSelect[bool]().Title("Set up a combined workflow for both code reviews and PR descriptions?").
		Options(
			huh.NewOption("Yes", true).Selected(true),
			huh.NewOption("No", false),
		)

	selectStructuredWorkflowInput.WithKeyMap(huh.NewDefaultKeyMap())
	selectStructuredWorkflowInput.Height(3)
	selectStructuredWorkflowInput.Blur()
	selectStructuredWorkflowInput.WithTheme(styles.HuhThemeCatppuccin{Styles: appStyles})

	confirmDirectoryInput := huh.NewSelect[bool]().Title("Create `.bark/` directory with default instruction files?").
		Options(
			huh.NewOption("Yes", true).Selected(true),
			huh.NewOption("No", false),
		)

	confirmDirectoryInput.WithKeyMap(huh.NewDefaultKeyMap())
	confirmDirectoryInput.Height(3)
	confirmDirectoryInput.Blur()

	confirmDirectoryInput.WithTheme(styles.HuhThemeCatppuccin{Styles: appStyles})

	return ActionsModel{
		config:                        cfg,
		view:                          actionsWorkflowView,
		multiselectWorkflowInput:      multiselectWorkflowInput,
		selectStructuredWorkflowInput: selectStructuredWorkflowInput,
		summary:                       newActionsWorkflowSummaryModel(cfg),
		confirmDirectoryInput:         confirmDirectoryInput,
		styles:                        appStyles,
		isDarkMode:                    isDarkMode,
	}
}

func (m *ActionsModel) setSize(width, height int) {
	m.width = width
	m.height = height
	if m.view == actionsWorkflowSummaryView {
		m.summary.setSize(width, height)
	}
}

func (m ActionsModel) Init() tea.Cmd {
	return nil
}

func (m ActionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.setSize(msg.Width, msg.Height)

	case actionsSavedMsg:
		return m, tea.Quit

	case actionsSaveErrorMsg:
		m.saveError = msg.err
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "esc":
			switch m.view {
			case actionsWorkflowStructureView:
				m.view = actionsWorkflowView
				m.selectStructuredWorkflowInput.Blur()
				m.multiselectWorkflowInput.Focus()
				return m, nil
			case actionsWorkflowSummaryView:
				if m.summary.shouldPreventExit() {
					break
				}

				options := m.selectedWorkflowOptions
				if len(options) == 1 {
					m.view = actionsWorkflowView
				} else {
					m.view = actionsWorkflowStructureView
				}
				m.selectStructuredWorkflowInput.Focus()
				return m, nil

			case actionsConfirmDirectoryView:
				m.view = actionsWorkflowSummaryView
				m.confirmDirectoryInput.Blur()
				return m, nil

			case actionsSaveView:
				m.saveError = nil
				m.view = actionsConfirmDirectoryView
				m.confirmDirectoryInput.Focus()
				return m, nil
			}

		case "enter":
			switch m.view {
			case actionsWorkflowView:
				values := m.multiselectWorkflowInput.GetValue().([]actionsWorkflowOption)
				if len(values) > 0 {
					m.selectedWorkflowOptions = values
					m.multiselectWorkflowInput.Blur()

					if len(values) == 1 {
						m.combinedWorkflow = false
						m.setWorkflowsSummary()
					} else {
						m.selectStructuredWorkflowInput.Focus()
						m.view = actionsWorkflowStructureView
					}
				}

			case actionsWorkflowStructureView:
				m.combinedWorkflow = m.selectStructuredWorkflowInput.GetValue().(bool)
				m.setWorkflowsSummary()

			case actionsWorkflowSummaryView:
				if m.summary.shouldPreventExit() {
					break
				}
				m.confirmDirectoryInput.Focus()
				m.view = actionsConfirmDirectoryView

			case actionsConfirmDirectoryView:
				m.view = actionsSaveView

			case actionsSaveView:
				m.saveError = nil
				return m, m.saveFiles()
			}
		}
	}

	switch m.view {
	case actionsWorkflowView:
		m.multiselectWorkflowInput.Focus()
		multiselectWorflowInput, cmd := m.multiselectWorkflowInput.Update(msg)
		m.multiselectWorkflowInput = multiselectWorflowInput.(*huh.MultiSelect[actionsWorkflowOption])
		cmds = append(cmds, cmd)

	case actionsWorkflowStructureView:
		selectStructuredWorkflowInput, cmd := m.selectStructuredWorkflowInput.Update(msg)
		m.selectStructuredWorkflowInput = selectStructuredWorkflowInput.(*huh.Select[bool])
		cmds = append(cmds, cmd)

	case actionsWorkflowSummaryView:
		summary, cmd := m.summary.Update(msg)
		m.summary = summary
		cmds = append(cmds, cmd)

	case actionsConfirmDirectoryView:
		confirmDirectoryInput, cmd := m.confirmDirectoryInput.Update(msg)
		m.confirmDirectoryInput = confirmDirectoryInput.(*huh.Select[bool])
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m ActionsModel) View() tea.View {
	view := tea.NewView(m.createView())
	view.AltScreen = true
	view.WindowTitle = "Bark Actions Setup"

	return view
}

func (m ActionsModel) createView() string {
	switch m.view {
	case actionsWorkflowView:
		return viewMargin.Render(m.multiselectWorkflowInput.View() + "\n\n" + m.renderHelp(true))
	case actionsWorkflowStructureView:
		return viewMargin.Render(
			m.selectStructuredWorkflowInput.View() + "\n\n" + m.renderHelp(false),
		)

	case actionsWorkflowSummaryView:
		return m.summary.View()

	case actionsConfirmDirectoryView:
		return viewMargin.Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				m.confirmDirectoryInput.View(),
				"\n",
				m.styles.Overlay1.Render(
					styles.Wrap(
						min(m.width-4, 80),
						"Creates reviewer.md, review.md, and pr.md with default templates. Without these, bark uses built-in defaults for all instructions and the reviewer persona.",
					),
				),
				"\n",
				m.renderHelp(false),
			),
		)
	case actionsSaveView:
		return viewMargin.Render(m.renderSaveView())
	default:
		return ""
	}
}

func (m ActionsModel) renderSaveView() string {
	key := m.styles.Subtext0.Render
	desc := m.styles.Overlay1.Render

	var header string
	switch {
	case m.saveError != nil:
		header = m.styles.Error.Bold(true).Render(m.saveError.Error())
	default:
		header = m.styles.Primary.Bold(true).Render("Confirm & Save")
	}

	var tree strings.Builder
	tree.WriteString(m.styles.Subtext1.Render("  .github/workflows/"))
	tree.WriteString("\n")
	if m.summary.combinedWorkflow {
		tree.WriteString(m.styles.Text.Render("    bark.yaml"))
	} else {
		for i, opt := range m.selectedWorkflowOptions {
			tree.WriteString(m.styles.Text.Render("    " + opt.workflowFileName()))
			if i < len(m.selectedWorkflowOptions)-1 {
				tree.WriteString("\n")
			}
		}
	}

	if m.confirmDirectoryInput.GetValue().(bool) {
		tree.WriteString("\n\n")
		tree.WriteString(m.styles.Subtext1.Render("  .bark/"))
		tree.WriteString("\n")
		tree.WriteString(m.styles.Text.Render("    reviewer.md"))
		tree.WriteString("\n")
		tree.WriteString(m.styles.Text.Render("    review.md"))
		tree.WriteString("\n")
		tree.WriteString(m.styles.Text.Render("    pr.md"))
	}

	var help string
	switch {
	case m.saveError != nil:
		help = key("enter") + desc(" retry")
		help += desc(" • ") + key("esc") + desc(" back")
	default:
		help = key("enter") + desc(" save")
		help += desc(" • ") + key("esc") + desc(" back")
	}
	help += desc(" • ") + key("ctrl+c") + desc(" quit")

	return lipgloss.JoinVertical(lipgloss.Left, header, "", tree.String(), "", help)
}

func (m ActionsModel) saveFiles() tea.Cmd {
	opts := scaffold.Options{
		Workflows:     m.summary.workflows,
		CreateBarkDir: m.confirmDirectoryInput.GetValue().(bool),
	}

	return func() tea.Msg {
		if err := scaffold.Run(opts); err != nil {
			return actionsSaveErrorMsg{err}
		}

		return actionsSavedMsg{}
	}
}

func (m *ActionsModel) setWorkflowsSummary() {
	m.summary.setStyles(m.styles, true)
	m.summary.setWorkflows(m.selectedWorkflowOptions, m.combinedWorkflow)
	m.summary.setSize(m.width, m.height)
	m.view = actionsWorkflowSummaryView
}

func (m *ActionsModel) renderHelp(multiselect bool) string {
	key := m.styles.Subtext0.Render
	desc := m.styles.Overlay1.Render

	help := key("↑/k") + desc(" up")
	help += desc(" • ") + key("↓/j") + desc(" down")
	if multiselect {
		help += desc(" • ") + key("space") + desc(" select/deselect")
	}
	help += desc(" • ") + key("enter") + desc(" select")
	help += desc(" • ") + key("esc") + desc(" back")
	help += desc(" • ") + key("ctrl+c") + desc(" quit")

	return help
}

type actionsWorkflowSummaryModel struct {
	width, height int
	styles        styles.Styles
	config        config.Config

	editor           editor.Model
	list             list.Model
	workflows        map[string]string
	editorFocused    bool
	combinedWorkflow bool
	selectedOptions  []actionsWorkflowOption
	error            error
}

func newActionsWorkflowSummaryModel(cfg config.Config) actionsWorkflowSummaryModel {
	textEditor := editor.New(80, 24)
	textEditor.SetExtraHighlightedContextLines(500)
	textEditor.DisableCommandMode(true)

	return actionsWorkflowSummaryModel{
		config:    cfg,
		editor:    textEditor,
		workflows: make(map[string]string),
	}
}

func (m *actionsWorkflowSummaryModel) setStyles(s styles.Styles, isDarkMode bool) {
	m.styles = s
	m.list = newListModel("", m.list.Items(), s, isDarkMode)
	m.list.SetFilteringEnabled(false)
	m.list.SetShowStatusBar(false)
	m.list.SetShowHelp(false)

	m.editor.WithTheme(styles.EditorTheme(s))
	m.editor.SetLanguage("yaml", styles.EditorLanguageTheme(isDarkMode))
}

func (m *actionsWorkflowSummaryModel) setWorkflows(options []actionsWorkflowOption, combinedWorkflow bool) {
	m.combinedWorkflow = combinedWorkflow
	m.editorFocused = combinedWorkflow || len(options) == 1
	m.selectedOptions = options

	items := []list.Item{}
	if combinedWorkflow {
		if _, ok := m.workflows["bark.yaml"]; !ok {
			m.workflows["bark.yaml"] = templates.GetDefaultCombinedActionTemplate()
		}
	} else {
		for _, workflow := range options {
			var title, key string

			switch workflow {
			case reviewWorkflow:
				title = "Code review"
				key = reviewWorkflow.workflowFileName()
				if _, ok := m.workflows[key]; !ok {
					m.workflows[key] = templates.GetDefaultReviewActionTemplate()
				}

			case prDescriptionWorkflow:
				title = "PR description"
				key = prDescriptionWorkflow.workflowFileName()
				if _, ok := m.workflows[key]; !ok {
					m.workflows[key] = templates.GetDefaultPRDescriptionActionTemplate()
				}
			}

			items = append(items, item{
				title: title,
				key:   key,
			})
		}
	}

	m.list.SetItems(items)

	firstKey := options[0].workflowFileName()
	if combinedWorkflow {
		firstKey = "bark.yaml"
	}
	m.editor.SetContent(m.workflows[firstKey])

	if m.editorFocused {
		m.editor.Focus()
	}
}

func (m *actionsWorkflowSummaryModel) setSize(width, height int) {
	m.width = width
	m.height = height
	m.list.SetSize(actionsListOuterWidth-2, height-4)

	if m.combinedWorkflow || len(m.selectedOptions) == 1 {
		m.editor.SetSize(width-6, height-5)
	} else {
		m.editor.SetSize(width-actionsListOuterWidth-4, height-5)
	}
}

func (m actionsWorkflowSummaryModel) shouldPreventExit() bool {
	return m.editor.IsInsertMode() || m.editor.IsSearchMode()
}

func (m actionsWorkflowSummaryModel) Init() tea.Cmd {
	return nil
}

func (m actionsWorkflowSummaryModel) Update(msg tea.Msg) (actionsWorkflowSummaryModel, tea.Cmd) {
	switch msg := msg.(type) {
	case actionsExternalEditorMsg:
		if msg.err != nil {
			m.error = msg.err
			return m, utils.DispatchClearMsg(5 * time.Second)
		}

		m.editor.SetBytes(msg.content)

		if selected, ok := m.list.SelectedItem().(item); ok {
			m.workflows[selected.key] = m.editor.GetCurrentContent()
		} else if m.combinedWorkflow {
			m.workflows["bark.yaml"] = m.editor.GetCurrentContent()
		}

	case utils.ClearMsg:
		m.error = nil

	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			if m.combinedWorkflow || len(m.selectedOptions) == 1 {
				break
			}

			if !m.editor.IsInsertMode() {
				m.editorFocused = !m.editorFocused
			}

			if m.editorFocused {
				m.editor.Focus()
			} else {
				m.editor.Blur()
			}

		case "ctrl+e":
			if !m.shouldPreventExit() {
				return m, m.openInEditor()
			}
		}
	}

	var cmds []tea.Cmd

	if m.editorFocused {
		if m.editor.HasChanges() {
			if item, ok := m.list.SelectedItem().(item); ok {
				m.workflows[item.key] = m.editor.GetCurrentContent()
			} else if m.combinedWorkflow {
				m.workflows["bark.yaml"] = m.editor.GetCurrentContent()
			}
		}

		editor, cmd := m.editor.Update(msg)
		m.editor = editor

		cmds = append(cmds, cmd)
	} else {
		list, cmd := m.list.Update(msg)
		m.list = list
		cmds = append(cmds, cmd)
		selected := m.list.SelectedItem()
		if item, ok := selected.(item); ok {
			if content, ok := m.workflows[item.key]; ok {
				m.editor.SetContent(content)
				_ = m.editor.SetCursorPosition(0, 0)
				m.editor, _ = m.editor.Update(nil)
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m actionsWorkflowSummaryModel) View() string {
	title := lipgloss.NewStyle().MarginLeft(2).Render(
		m.styles.Primary.Bold(true).Render("Preview & Edit Workflows"),
	)

	leftContent := m.list.View()
	var leftBorder lipgloss.Style
	if !m.editorFocused {
		leftBorder = m.styles.ActiveBorder
	} else {
		leftBorder = m.styles.InactiveBorder
	}
	leftPanel := leftBorder.Width(actionsListOuterWidth - 2).Render(leftContent)

	var filenameStr string
	if selected, ok := m.list.SelectedItem().(item); ok {
		filenameStr = m.styles.Subtext0.PaddingLeft(1).Render(".github/workflows/" + selected.key)
	} else {
		filenameStr = m.styles.Subtext0.PaddingLeft(1).Render("bark.yaml")
	}

	editorContent := lipgloss.JoinVertical(lipgloss.Left, filenameStr, m.editor.View())

	rightWidth := m.width - actionsListOuterWidth - 2
	if m.combinedWorkflow || len(m.selectedOptions) == 1 {
		rightWidth = m.width - 4
	}

	var rightBorder lipgloss.Style
	if m.editorFocused {
		rightBorder = m.styles.ActiveBorder
	} else {
		rightBorder = m.styles.InactiveBorder
	}
	rightPanel := rightBorder.Width(rightWidth).Render(editorContent)

	content := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, " ", rightPanel)

	if m.combinedWorkflow || len(m.selectedOptions) == 1 {
		content = rightPanel
	}

	help := lipgloss.NewStyle().MarginLeft(2).Render(m.renderHelp())

	return lipgloss.NewStyle().Margin(0, 2).Width(m.width).Render(
		lipgloss.JoinVertical(lipgloss.Left, title, content, help),
	)
}

func (m actionsWorkflowSummaryModel) renderHelp() string {
	if m.error != nil {
		return m.styles.Error.Bold(true).Render("Error: " + m.error.Error())
	}

	key := m.styles.Subtext0.Render
	desc := m.styles.Overlay1.Render

	var help string
	if m.editorFocused {
		if !m.editor.IsInsertMode() {
			help = key("i") + desc(" edit")
		} else {
			help = key("esc") + desc(" stop editing")
		}

		if len(m.workflows) > 1 && !m.editor.IsInsertMode() {
			help += desc(" • ") + key("tab") + desc(" switch to list")
		}
	} else {
		help = key("↑/k") + desc(" up")
		help += desc(" • ") + key("↓/j") + desc(" down")
		help += desc(" • ") + key("tab") + desc(" switch to editor")
	}

	if !m.editor.IsInsertMode() {
		help += desc(" • ") + key("enter") + desc(" next")
	}
	help += desc(" • ") + key("ctrl+c") + desc(" quit")

	return help
}

func (m actionsWorkflowSummaryModel) openInEditor() tea.Cmd {
	tmpFile, err := os.CreateTemp(os.TempDir(), "*.yaml")
	if err != nil {
		return utils.DispatchMsg(actionsExternalEditorMsg{err: err})
	}

	if _, err = tmpFile.WriteString(m.editor.GetCurrentContent()); err != nil {
		return utils.DispatchMsg(actionsExternalEditorMsg{err: err})
	}

	cmd, err := utils.OpenInEditorCmd(m.config.GetEditor(), tmpFile.Name())
	if err != nil {
		return utils.DispatchMsg(actionsExternalEditorMsg{err: err})
	}

	return tea.ExecProcess(cmd, func(error) tea.Msg {
		content, err := os.ReadFile(tmpFile.Name())
		_ = os.Remove(tmpFile.Name())
		return actionsExternalEditorMsg{content: content, err: err}
	})
}
