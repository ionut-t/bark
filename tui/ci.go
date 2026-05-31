package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/templates"
	"github.com/ionut-t/bark/v2/internal/utils"
	"github.com/ionut-t/bark/v2/pkg/reviewers"
	"github.com/ionut-t/coffee/styles"
	editor "github.com/ionut-t/goeditor"
)

type cancelCIWorkflowSelectionMsg struct{}

type ciExternalEditorMsg struct {
	content []byte
	err     error
}

type (
	ciSavedMsg     struct{}
	ciSaveErrorMsg struct{ err error }
)

type ciView int

const (
	ciWorkflowView ciView = iota
	ciWorkflowStructureView
	ciWorkflowSummaryView
	ciConfirmDirectoryView
	ciSaveView
)

const defaultReviewInstructions = `You are a senior software engineer conducting a thorough code review.

## Review Focus

- **Correctness**: Logic errors, edge cases, and potential bugs
- **Security**: Vulnerabilities and unsafe practices
- **Maintainability**: Overly complex or brittle code
- **Performance**: Obvious inefficiencies worth addressing
- **Conventions**: Language idioms and project-specific patterns

Be specific and actionable. Explain why a change is needed, not just what to change.
`

type ciWorkflowOption int

const (
	reviewWorkflow ciWorkflowOption = iota
	prDescriptionWorkflow
)

func (o ciWorkflowOption) String() string {
	switch o {
	case reviewWorkflow:
		return "Set up CI for code reviews"
	case prDescriptionWorkflow:
		return "Set up CI for PR descriptions"
	default:
		return ""
	}
}

func (o ciWorkflowOption) WorkflowFileName() string {
	switch o {
	case reviewWorkflow:
		return "bark-review.yaml"
	case prDescriptionWorkflow:
		return "bark-pr-description.yaml"
	default:
		return "bark"
	}
}

type ciModel struct {
	width, height int
	config        config.Config

	view   ciView
	styles styles.Styles

	selectedWorkflowOptions []ciWorkflowOption
	combinedWorkflow        bool

	multiselectWorflowInput       *huh.MultiSelect[ciWorkflowOption]
	selectStructuredWorkflowInput *huh.Select[bool]
	confirmDirectoryInput         *huh.Select[bool]

	summary   ciWorkflowSummaryModel
	saveError error
}

func newCIModel(cfg config.Config) ciModel {
	multiselectWorkflowInput := huh.NewMultiSelect[ciWorkflowOption]().Title("Select CI workflows to set up").
		Options(
			huh.NewOption(reviewWorkflow.String(), reviewWorkflow),
			huh.NewOption(prDescriptionWorkflow.String(), prDescriptionWorkflow),
		)

	multiselectWorkflowInput.WithKeyMap(huh.NewDefaultKeyMap())
	multiselectWorkflowInput.Focus()
	multiselectWorkflowInput.Height(3)

	selectStructuredWorkflowInput := huh.NewSelect[bool]().Title("Set up a combined workflow for both code reviews and PR descriptions?").
		Options(
			huh.NewOption("Yes", true).Selected(true),
			huh.NewOption("No", false),
		)

	selectStructuredWorkflowInput.WithKeyMap(huh.NewDefaultKeyMap())
	selectStructuredWorkflowInput.Height(3)
	selectStructuredWorkflowInput.Blur()

	selectDirectoryInput := huh.NewSelect[bool]().Title("Create `.bark/` directory with default instruction files?").
		Options(
			huh.NewOption("Yes", true).Selected(true),
			huh.NewOption("No", false),
		)

	selectDirectoryInput.WithKeyMap(huh.NewDefaultKeyMap())
	selectDirectoryInput.Height(3)
	selectDirectoryInput.Blur()

	return ciModel{
		config:                        cfg,
		view:                          ciWorkflowView,
		multiselectWorflowInput:       multiselectWorkflowInput,
		selectStructuredWorkflowInput: selectStructuredWorkflowInput,
		summary:                       newCIWorkflowSummaryModel(cfg),
		confirmDirectoryInput:         selectDirectoryInput,
	}
}

func (m *ciModel) setSize(width, height int) {
	m.width = width
	m.height = height
	if m.view == ciWorkflowSummaryView {
		m.summary.setSize(width, height)
	}
}

func (m *ciModel) setStyles(s styles.Styles) {
	m.styles = s
	m.multiselectWorflowInput.WithTheme(styles.HuhThemeCatppuccin{Styles: s})
	m.selectStructuredWorkflowInput.WithTheme(styles.HuhThemeCatppuccin{Styles: s})
	m.confirmDirectoryInput.WithTheme(styles.HuhThemeCatppuccin{Styles: s})
}

func (m ciModel) Init() tea.Cmd {
	return nil
}

func (m ciModel) Update(msg tea.Msg) (ciModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case ciSavedMsg:
		return m, tea.Quit

	case ciSaveErrorMsg:
		m.saveError = msg.err
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			switch m.view {
			case ciWorkflowView:
				return m, utils.DispatchMsg(cancelCIWorkflowSelectionMsg{})
			case ciWorkflowStructureView:
				m.view = ciWorkflowView
				m.selectStructuredWorkflowInput.Blur()
				m.multiselectWorflowInput.Focus()
				return m, nil
			case ciWorkflowSummaryView:
				if m.summary.shouldPreventExit() {
					break
				}

				options := m.selectedWorkflowOptions
				if len(options) == 1 {
					m.view = ciWorkflowView
				} else {
					m.view = ciWorkflowStructureView
				}
				m.selectStructuredWorkflowInput.Focus()
				return m, nil

			case ciConfirmDirectoryView:
				m.view = ciWorkflowSummaryView
				m.confirmDirectoryInput.Blur()
				return m, nil

			case ciSaveView:
				m.saveError = nil
				m.view = ciConfirmDirectoryView
				m.confirmDirectoryInput.Focus()
				return m, nil
			}

		case "enter":
			switch m.view {
			case ciWorkflowView:
				values := m.multiselectWorflowInput.GetValue().([]ciWorkflowOption)
				if len(values) > 0 {
					m.selectedWorkflowOptions = values
					m.multiselectWorflowInput.Blur()

					if len(values) == 1 {
						m.setWorkflowsSummary()
					} else {
						m.selectStructuredWorkflowInput.Focus()
						m.view = ciWorkflowStructureView
					}
				}

			case ciWorkflowStructureView:
				m.setWorkflowsSummary()

			case ciWorkflowSummaryView:
				if m.summary.shouldPreventExit() {
					break
				}
				m.confirmDirectoryInput.Focus()
				m.view = ciConfirmDirectoryView

			case ciConfirmDirectoryView:
				m.view = ciSaveView

			case ciSaveView:
				m.saveError = nil
				return m, m.saveFiles()
			}
		}
	}

	switch m.view {
	case ciWorkflowView:
		m.multiselectWorflowInput.Focus()
		multiselectWorflowInput, cmd := m.multiselectWorflowInput.Update(msg)
		m.multiselectWorflowInput = multiselectWorflowInput.(*huh.MultiSelect[ciWorkflowOption])
		cmds = append(cmds, cmd)

	case ciWorkflowStructureView:
		selectStructuredWorkflowInput, cmd := m.selectStructuredWorkflowInput.Update(msg)
		m.selectStructuredWorkflowInput = selectStructuredWorkflowInput.(*huh.Select[bool])
		cmds = append(cmds, cmd)

	case ciWorkflowSummaryView:
		summary, cmd := m.summary.Update(msg)
		m.summary = summary
		cmds = append(cmds, cmd)

	case ciConfirmDirectoryView:
		confirmDirectoryInput, cmd := m.confirmDirectoryInput.Update(msg)
		m.confirmDirectoryInput = confirmDirectoryInput.(*huh.Select[bool])
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m ciModel) View() string {
	switch m.view {
	case ciWorkflowView:

		return viewMargin.Render(m.multiselectWorflowInput.View() + "\n\n" + m.renderHelp(true))
	case ciWorkflowStructureView:
		return viewMargin.Render(
			m.selectStructuredWorkflowInput.View() + "\n\n" + m.renderHelp(false),
		)

	case ciWorkflowSummaryView:
		return m.summary.View()

	case ciConfirmDirectoryView:
		return viewMargin.Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				m.confirmDirectoryInput.View(),
				"\n",
				m.styles.Overlay1.Render(
					styles.Wrap(
						min(m.width-4, 80),
						"Creates reviewer.md, review.md, and pr.md with default templates. Without these, bark falls back to built-in defaults and requires BARK_REVIEWER to be set.",
					),
				),
				"\n",
				m.renderHelp(false),
			),
		)
	case ciSaveView:
		return viewMargin.Render(m.renderSaveView())
	default:
		return ""
	}
}

func (m ciModel) renderSaveView() string {
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
			tree.WriteString(m.styles.Text.Render("    " + opt.WorkflowFileName()))
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

func (m ciModel) saveFiles() tea.Cmd {
	workflows := m.summary.workflows
	createBarkDir := m.confirmDirectoryInput.GetValue().(bool)

	return func() tea.Msg {
		if err := os.MkdirAll(filepath.Join(".github", "workflows"), 0o755); err != nil {
			return ciSaveErrorMsg{err}
		}

		for filename, content := range workflows {
			if err := os.WriteFile(filepath.Join(".github", "workflows", filename), []byte(content), 0o644); err != nil {
				return ciSaveErrorMsg{err}
			}
		}

		if createBarkDir {
			if err := os.MkdirAll(".bark", 0o755); err != nil {
				return ciSaveErrorMsg{err}
			}

			reviewer, err := reviewers.GetEmbedded("Linus Torvalds")
			if err != nil {
				return ciSaveErrorMsg{err}
			}

			barkFiles := map[string]string{
				"reviewer.md": reviewer.Prompt,
				"review.md":   defaultReviewInstructions,
				"pr.md":       templates.GetDefaultPRInstructions(),
			}
			for filename, content := range barkFiles {
				if err := os.WriteFile(filepath.Join(".bark", filename), []byte(content), 0o644); err != nil {
					return ciSaveErrorMsg{err}
				}
			}
		}

		return ciSavedMsg{}
	}
}

func (m *ciModel) setWorkflowsSummary() {
	m.combinedWorkflow = m.selectStructuredWorkflowInput.GetValue().(bool)
	m.summary.setStyles(m.styles, true)
	m.summary.setWorkflows(m.selectedWorkflowOptions, m.combinedWorkflow)
	m.summary.setSize(m.width, m.height)
	m.view = ciWorkflowSummaryView
}

func (m *ciModel) renderHelp(multiselect bool) string {
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

const ciListOuterWidth = 38

type ciWorkflowSummaryModel struct {
	width, height int
	styles        styles.Styles
	config        config.Config

	editor           editor.Model
	list             list.Model
	workflows        map[string]string
	editorFocused    bool
	combinedWorkflow bool
	error            error
}

func newCIWorkflowSummaryModel(cfg config.Config) ciWorkflowSummaryModel {
	textEditor := editor.New(80, 24)
	textEditor.SetExtraHighlightedContextLines(500)
	textEditor.DisableCommandMode(true)

	return ciWorkflowSummaryModel{
		config:    cfg,
		editor:    textEditor,
		workflows: make(map[string]string),
	}
}

func (m *ciWorkflowSummaryModel) setStyles(s styles.Styles, isDarkMode bool) {
	m.styles = s
	m.list = newListModel("", m.list.Items(), s, isDarkMode)
	m.list.SetFilteringEnabled(false)
	m.list.SetShowStatusBar(false)
	m.list.SetShowHelp(false)

	m.editor.WithTheme(styles.EditorTheme(s))
	m.editor.SetLanguage("yaml", styles.EditorLanguageTheme(isDarkMode))
}

func (m *ciWorkflowSummaryModel) setWorkflows(options []ciWorkflowOption, combinedWorkflow bool) {
	m.combinedWorkflow = combinedWorkflow || len(options) == 1
	m.editorFocused = m.combinedWorkflow

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
				key = reviewWorkflow.WorkflowFileName()
				if _, ok := m.workflows[key]; !ok {
					m.workflows[key] = templates.GetDefaultReviewActionTemplate()
				}

			case prDescriptionWorkflow:
				title = "PR description"
				key = prDescriptionWorkflow.WorkflowFileName()
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

	firstKey := options[0].WorkflowFileName()
	if combinedWorkflow {
		firstKey = "bark.yaml"
	}
	m.editor.SetContent(m.workflows[firstKey])

	if m.editorFocused {
		m.editor.Focus()
	}
}

func (m *ciWorkflowSummaryModel) setSize(width, height int) {
	m.width = width
	m.height = height
	m.list.SetSize(ciListOuterWidth-2, height-4)

	if m.combinedWorkflow {
		m.editor.SetSize(width-6, height-5)
	} else {
		m.editor.SetSize(width-ciListOuterWidth-4, height-5)
	}
}

func (m ciWorkflowSummaryModel) shouldPreventExit() bool {
	return m.editor.IsInsertMode() || m.editor.IsSearchMode()
}

func (m ciWorkflowSummaryModel) Init() tea.Cmd {
	return nil
}

func (m ciWorkflowSummaryModel) Update(msg tea.Msg) (ciWorkflowSummaryModel, tea.Cmd) {
	switch msg := msg.(type) {
	case ciExternalEditorMsg:
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
			if m.combinedWorkflow {
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
		if item, ok := m.list.SelectedItem().(item); ok {
			m.workflows[item.key] = m.editor.GetCurrentContent()
		} else if m.combinedWorkflow {
			m.workflows["bark.yaml"] = m.editor.GetCurrentContent()
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

func (m ciWorkflowSummaryModel) View() string {
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
	leftPanel := leftBorder.Width(ciListOuterWidth - 2).Render(leftContent)

	var filenameStr string
	if selected, ok := m.list.SelectedItem().(item); ok {
		filenameStr = m.styles.Subtext0.PaddingLeft(1).Render(".github/workflows/" + selected.key)
	}
	editorContent := lipgloss.JoinVertical(lipgloss.Left, filenameStr, m.editor.View())

	rightWidth := m.width - ciListOuterWidth - 2
	if m.combinedWorkflow {
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

	if m.combinedWorkflow {
		content = rightPanel
	}

	help := lipgloss.NewStyle().MarginLeft(2).Render(m.renderHelp())

	return lipgloss.NewStyle().Margin(0, 2).Width(m.width).Render(
		lipgloss.JoinVertical(lipgloss.Left, title, content, help),
	)
}

func (m ciWorkflowSummaryModel) renderHelp() string {
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

func (m ciWorkflowSummaryModel) openInEditor() tea.Cmd {
	tmpFile, err := os.CreateTemp(os.TempDir(), "*.yaml")
	if err != nil {
		return utils.DispatchMsg(ciExternalEditorMsg{err: err})
	}

	if _, err = tmpFile.WriteString(m.editor.GetCurrentContent()); err != nil {
		return utils.DispatchMsg(ciExternalEditorMsg{err: err})
	}

	return tea.ExecProcess(exec.Command(m.config.GetEditor(), tmpFile.Name()), func(error) tea.Msg {
		content, err := os.ReadFile(tmpFile.Name())
		_ = os.Remove(tmpFile.Name())
		return ciExternalEditorMsg{content: content, err: err}
	})
}
