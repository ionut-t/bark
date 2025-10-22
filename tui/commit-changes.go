package tui

import (
	"context"
	"errors"
	"slices"

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
	width, height int

	editor           editor.Model
	commitAll        bool
	loading          bool
	loadingMsg       string
	loadingMsgPicker *loadingMessagePicker
	spinner          spinner.Model
	llm              llm.LLM
	prompt           string
	error            error
	response         string
	isShowingPrompt  bool
}

type commitChangesMsg struct {
	message   string
	commitAll bool
}

func newCommitChangesModel(llm llm.LLM, prompt string, commitAll bool, width, height int) commitChangesModel {
	textEditor := editor.New(width, height)
	textEditor.DisableCommandMode(true)
	textEditor.WithTheme(styles.EditorTheme())
	textEditor.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styles.Primary

	m := commitChangesModel{
		width:            width,
		height:           height,
		editor:           textEditor,
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
	m.width = width
	m.height = height
}

func (m commitChangesModel) Init() tea.Cmd {
	return nil
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
		m.isShowingPrompt = false

		if msg.error != nil {
			// Check if it's a context cancellation - don't show as error
			if errors.Is(msg.error, context.Canceled) {
				m.error = nil
				return m, nil
			}
			m.error = msg.error
			return m, nil
		}

		m.response = utils.RemoveCodeFences(msg.message)
		m.editor.SetContent(m.response)
		m.editor.SetSize(m.width-4, max(10, m.height-lipgloss.Height(m.header())-1))

	case editor.SearchResultsMsg:
		if len(msg.Positions) == 0 {
			return m, DispatchNoSearchResultsError(&m.editor)
		}

	case tea.KeyMsg:
		if m.loading {
			// Don't process key events while loading
			return m, nil
		}

		switch msg.String() {
		case "tab":
			if !m.editor.IsNormalMode() {
				break
			}

			m.isShowingPrompt = !m.isShowingPrompt
			if m.isShowingPrompt {
				m.editor.SetContent(m.prompt + "\n")
				m.editor.SetLanguage("markdown", styles.HighlighterTheme())
				m.editor.SetExtraHighlightedContextLines(300)
			} else {
				m.editor.SetContent(m.response)
				m.editor.SetLanguage("", "")
			}

		case "alt+enter", "ctrl+s":
			m.loading = true
			m.loadingMsg = "Committing changes..."
			return m, tea.Batch(
				m.spinner.Tick,
				utils.DispatchMsg(
					commitChangesMsg{
						message:   m.editor.GetCurrentContent(),
						commitAll: m.commitAll,
					},
				),
			)
		}
	}

	ed, cmd := m.editor.Update(msg)
	m.editor = ed.(editor.Model)

	if m.isShowingPrompt {
		m.prompt = m.editor.GetCurrentContent()
	}

	return m, cmd
}

func (m commitChangesModel) View() string {
	if m.loading {
		return lipgloss.NewStyle().Padding(2, 2, 0).Render(lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.spinner.View(),
			styles.Accent.Render(" "+m.loadingMsg),
		))
	}

	header := m.header()
	headerHeight := lipgloss.Height(header)
	editorHeight := m.height - headerHeight - 1

	m.editor.SetSize(m.width-4, max(10, editorHeight))

	return lipgloss.NewStyle().Padding(0, 2).Render(lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		m.editor.View(),
	))
}

func (m *commitChangesModel) header() string {
	var header string
	if m.error != nil {
		err := styles.Wrap(80, styles.Error.Render("Error: "+m.error.Error()))
		header = styles.Subtext0.Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				err,
				"\n\n",
				"Press r to retry",
			),
		)
	} else {
		height := 7

		if m.isShowingPrompt {
			height = 6
		}

		header = lipgloss.NewStyle().Height(height).Render(m.commitChangesHelp())
	}

	border := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderBottomForeground(styles.Primary.GetForeground())

	return border.Render(header) + "\n"
}

func (m *commitChangesModel) commitChangesHelp() string {
	commands := []struct {
		Command     string
		Description string
	}{
		{"i", "edit commit message"},
		{"alt+enter/ctrl+s", "submit commit message"},
		{"tab", "view prompt"},
		{"ctrl+r", "generate a new commit message"},
		{"ctrl+c", "quit"},
	}

	if m.isShowingPrompt {
		commands = []struct {
			Command     string
			Description string
		}{
			{"i", "edit prompt"},
			{"tab", "view commit message"},
			{"ctrl+r", "generate a new commit message"},
			{"ctrl+c", "quit"},
		}
	}

	if m.editor.IsInsertMode() {
		commands = slices.DeleteFunc(
			commands, func(c struct {
				Command     string
				Description string
			}) bool {
				return c.Command == "i" || c.Command == "ctrl+r" || c.Command == "tab"
			},
		)

		commands = slices.Insert(commands, 0, struct {
			Command     string
			Description string
		}{"esc", "exit insert mode"},
		)
	}

	return help.RenderCmdHelp(m.width, commands)
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

func (m *commitChangesModel) canRetry() bool {
	return !m.loading && m.editor.IsNormalMode()
}
