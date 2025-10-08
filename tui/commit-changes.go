package tui

import (
	"context"
	"errors"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ionut-t/bark/internal/utils"
	"github.com/ionut-t/bark/pkg/llm"
	"github.com/ionut-t/coffee/help"
	"github.com/ionut-t/coffee/styles"
	editor "github.com/ionut-t/goeditor/adapter-bubbletea"
)

var commitLoadingMessages = []string{
	// Classic commit message struggles
	"Crafting the perfect commit message...",
	"Avoiding 'fix stuff' and 'updated files'...",
	"Resisting the urge to write 'WIP'...",
	"Trying not to write 'minor changes'...",
	"Suppressing 'fixed bug' instincts...",
	"Channeling conventional commit energy...",

	// Git humor
	"git commit -m 'AI did it'...",
	"Blaming previous commits...",
	"Rewriting git history... wait, wrong feature...",
	"Squashing your shame into one commit...",
	"Cherry-picking the best words...",
	"Rebasing your expectations...",

	// Professional vs reality
	"Converting 'idk why this works' to professional speak...",
	"Translating 'YOLO' into corporate...",
	"Making 'it works now' sound intentional...",
	"Disguising trial and error as strategy...",
	"Framing luck as expertise...",

	// Semantic commit types
	"Deciding between fix, feat, or chore...",
	"Adding appropriate emoji... just kidding...",
	"Determining if this is breaking or not...",
	"Classifying your chaos...",

	// Code change analysis
	"Analyzing what you actually changed...",
	"Figuring out what this diff means...",
	"Reading your mind (and your code)...",
	"Decoding the intent behind these changes...",
	"Understanding what future you will need to know...",

	// Time and process
	"Taking longer than the actual code changes...",
	"Spending more time on message than code...",
	"Practicing the art of summarization...",
	"Condensing hours of work into 50 characters...",

	// Playful/Meta
	"Writing commit message for commit message generator...",
	"Committing to writing better commits...",
	"Meta-analyzing your changes...",
	"Generating commit inception...",

	// Developer habits
	"Checking if anyone will actually read this...",
	"Preparing for future blame annotations...",
	"Writing for your future self...",
	"Documenting for the archaeologists...",
	"Creating tomorrow's git log...",

	// Code quality themes
	"Summarizing your refactoring journey...",
	"Describing technical debt payments...",
	"Explaining the unexplainable...",
	"Justifying questionable decisions...",

	// AI/Creative
	"Consulting the commit message gods...",
	"Channeling Linus Torvalds...",
	"Computing semantic similarity...",
	"Training on 10 million commit messages...",
	"Avoiding passive voice...",

	// Short and punchy
	"Summarizing brilliance...",
	"Capturing context...",
	"Being concise...",
	"Staying under 72 characters...",
	"Making it meaningful...",
}

type commitLoadingMsg struct {
	message string
}

type commitResponseMsg struct {
	message string
	error   error
}

type commitChangesModel struct {
	editor           editor.Model
	commitAll        bool
	loading          bool
	loadingMsg       string
	loadingMsgPicker *loadingMessagePicker
	spinner          spinner.Model
	llm              llm.LLM
	prompt           string
	error            error
}

type commitChangesMsg struct {
	message   string
	commitAll bool
}

func newCommitChangesModel(llm llm.LLM, prompt string, commitAll bool) commitChangesModel {
	editorModel := editor.New(80, 20)
	editorModel.DisableCommandMode(true)
	editorModel.SetCursorBlinkMode(true)
	editorModel.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styles.Primary

	m := commitChangesModel{
		editor:           editorModel,
		spinner:          sp,
		loading:          true,
		llm:              llm,
		prompt:           prompt,
		commitAll:        commitAll,
		loadingMsgPicker: newLoadingMessagePicker(commitLoadingMessages),
	}

	m.loadingMsg = m.getLoadingMessage()
	return m
}

func (m *commitChangesModel) setSize(width, height int) {
	m.editor.SetSize(width-4, min(20, height))
}

func (m commitChangesModel) Init() tea.Cmd {
	return m.editor.CursorBlink()
}

func (m *commitChangesModel) startCommitGeneration(ctx context.Context) tea.Cmd {
	m.error = nil
	m.loading = true

	return tea.Batch(
		m.spinner.Tick,
		m.dispatchLoadingMsg(),
		getCommitMessage(ctx, m.llm, m.prompt),
	)
}

func (m commitChangesModel) Update(msg tea.Msg) (commitChangesModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case commitLoadingMsg:
		if !m.loading {
			return m, nil
		}

		m.loadingMsg = msg.message
		return m, m.dispatchLoadingMsg()

	case commitResponseMsg:
		m.loading = false

		if msg.error != nil {
			// Check if it's a context cancellation - don't show as error
			if errors.Is(msg.error, context.Canceled) {
				m.error = nil
				return m, nil
			}
			m.error = msg.error
			return m, nil
		}

		m.editor.SetContent(utils.RemoveCodeFences(msg.message))

	case tea.KeyMsg:
		if m.loading {
			// Don't process key events while loading
			return m, nil
		}

		switch msg.String() {
		case "alt+enter", "ctrl+s":
			return m, func() tea.Msg {
				return commitChangesMsg{
					message:   m.editor.GetCurrentContent(),
					commitAll: m.commitAll,
				}
			}
		}
	}

	ed, cmd := m.editor.Update(msg)
	m.editor = ed.(editor.Model)

	return m, cmd
}

func (m commitChangesModel) View() string {
	if m.loading {
		return lipgloss.NewStyle().Padding(2).Render(lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.spinner.View(),
			styles.Accent.Render(" "+m.loadingMsg),
		))
	}

	var errMsg string
	if m.error != nil {
		err := styles.Wrap(80, styles.Error.Render("Error: "+m.error.Error()))
		errMsg = styles.Subtext0.Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				"\n\n",
				err,
				"\n",
				"Press r to retry",
			),
		)
	}

	return lipgloss.NewStyle().Padding(2).Render(lipgloss.JoinVertical(
		lipgloss.Top,
		commitChangesHelp(80),
		"\n",
		lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Render(m.editor.View()),
		errMsg,
	))
}

func commitChangesHelp(width int) string {
	commands := []struct {
		Command     string
		Description string
	}{
		{"i", "edit commit message"},
		{"alt+enter/ctrl+s", "submit commit message"},
		{"ctrl+c", "quit"},
	}

	return help.RenderCmdHelp(width, commands)
}

func getCommitMessage(ctx context.Context, llm llm.LLM, prompt string) tea.Cmd {
	return func() tea.Msg {
		message, err := llm.Generate(ctx, prompt)
		return commitResponseMsg{
			message: message,
			error:   err,
		}
	}
}

func (m *commitChangesModel) dispatchLoadingMsg() tea.Cmd {
	return dispatchLoadingMessage(commitLoadingMsg{message: m.getLoadingMessage()})
}

func (m *commitChangesModel) getLoadingMessage() string {
	return m.loadingMsgPicker.next()
}
