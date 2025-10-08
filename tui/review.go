package tui

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ionut-t/bark/internal/utils"
	"github.com/ionut-t/bark/pkg/llm"
	"github.com/ionut-t/bark/pkg/reviewers"
	"github.com/ionut-t/coffee/help"
	"github.com/ionut-t/coffee/markdown"
	"github.com/ionut-t/coffee/styles"
	editor "github.com/ionut-t/goeditor/adapter-bubbletea"
)

var loadingMessages = []string{
	// Brewing/Cooking themed
	"Brewing the perfect review...",
	"Simmering your code with a dash of criticism...",
	"Marinating your commits in wisdom...",

	// Legendary/Epic themed
	"Summoning legendary reviewers...",
	"Channeling the wisdom of ancient code masters...",
	"Consulting the elder scroll of best practices...",

	// Brutally honest/Roasting themed
	"Analysing your code with ruthless precision...",
	"Preparing to unleash brutal honesty...",
	"Getting ready to roast your spaghetti code...",
	"Sharpening the critique katana...",
	"Loading the constructive criticism cannon...",
	"Calibrating the BS detector...",

	// Code quality themed
	"Searching for sneaky bugs in the shadows...",
	"Counting your TODO comments... this might take a while...",
	"Deciphering what this code actually does...",
	"Untangling your logic pretzels...",
	"Archaeological expedition into your codebase...",
	"Carbon dating your legacy code...",

	// AI/Processing themed
	"Training neural networks on your naming conventions...",
	"Computing the perfect roast-to-help ratio...",
	"Processing... complexity detected...",
	"Compiling witty remarks...",
	"Debugging your life choices...",

	// Developer culture references
	"Asking Stack Overflow for the best burns...",
	"Checking if your code passes the code smell test...",
	"Measuring technical debt in Bitcoin...",
	"Running static analysis on your life choices...",
	"Peer reviewing your peer review request...",

	// Time/Patience themed
	"Good things come to those who wait...",
	"Patience... greatness takes time...",
	"Rome wasn't debugged in a day...",
	"Quality reviews can't be rushed...",

	// Dramatic/Shakespearean
	"Crafting feedback with Shakespearean flair...",
	"To refactor or not to refactor...",
	"Penning your code's tragic backstory...",
	"Composing a sonnet about your spaghetti code...",

	// Playful/Cheeky
	"Judging your commit messages silently...",
	"Pretending to understand your regex...",
	"Wondering why you didn't use a switch statement...",
	"Contemplating your variable names...",
	"Taking deep breaths before reading this...",
	"Preparing emotionally for what's ahead...",
	"Trying not to cry...",

	// Action/Battle themed
	"Deploying the code critique squad...",
	"Assembling the review dream team...",
	"Mobilizing the refactoring task force...",
	"Unleashing the linter army...",

	// Mystery/Detective themed
	"Following the trail of code smells...",
	"Investigating suspicious patterns...",
	"Gathering evidence of anti-patterns...",
	"CSI: Code Scene Investigation...",

	// Scientific/Experimental
	"Conducting peer review experiments...",
	"Analysing under a microscope...",
	"Testing hypothesis: 'This will work'... unlikely...",
	"Peer reviewing in zero gravity...",
}

type streamReadyMsg struct {
	respChan <-chan llm.Response
	errChan  <-chan error
}

type streamErrorMsg struct {
	error error
}

type streamChunkMsg struct {
	content string
}

type streamCompleteMsg struct{}

type reviewLoadingMsg struct {
	message string
}

type reviewView int

const (
	reviewViewEditor reviewView = iota
	reviewViewMarkdown
)

type reviewModel struct {
	width, height    int
	editor           editor.Model
	llm              llm.LLM
	respChan         <-chan llm.Response
	errChan          <-chan error
	contentBuilder   *strings.Builder
	reviewer         reviewers.Reviewer
	prompt           string
	spinner          spinner.Model
	loading          bool
	loadingChunks    bool
	markdown         markdown.Model
	viewport         viewport.Model
	currentView      reviewView
	loadingMsg       string
	loadingMsgPicker *loadingMessagePicker
	error            error
}

func newReviewModel(reviewer reviewers.Reviewer, prompt string, width, height int, llm llm.LLM) reviewModel {
	textEditor := editor.New(width, height-1)
	textEditor.SetCursorMode(editor.CursorBlink)
	textEditor.SetLanguage("markdown", styles.HighlighterTheme())
	textEditor.DisableCommandMode(true)
	textEditor.DisableInsertMode(true)
	textEditor.SetExtraHighlightedContextLines(300)
	textEditor.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styles.Primary

	m := reviewModel{
		width:            width,
		height:           height,
		editor:           textEditor,
		llm:              llm,
		prompt:           prompt,
		loading:          true,
		loadingChunks:    true,
		spinner:          sp,
		markdown:         markdown.New(),
		viewport:         viewport.New(width, height),
		reviewer:         reviewer,
		loadingMsgPicker: newLoadingMessagePicker(loadingMessages),
	}

	m.loadingMsg = m.getLoadingMessage()

	return m
}

func (m *reviewModel) setSize(width, height int) {
	m.width = width
	m.height = height

	const statusBarHeight = 2
	contentHeight := height - statusBarHeight

	m.editor.SetSize(width, contentHeight)
	m.viewport.Height = contentHeight
	m.viewport.Width = width
}

func (m reviewModel) Init() tea.Cmd {
	return m.editor.CursorBlink()
}

func (m reviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case reviewLoadingMsg:
		if !m.loading {
			return m, nil
		}

		m.loadingMsg = msg.message
		return m, m.dispatchLoadingMsg()

	case streamReadyMsg:
		m.respChan = msg.respChan
		return m, watchStreamCmd(m.respChan, msg.errChan)

	case streamChunkMsg:
		if m.loading {
			m.spinner.Spinner = spinner.Points
		}

		m.loading = false

		m.contentBuilder.WriteString(msg.content)
		content := utils.NormaliseCodeFences(m.contentBuilder.String())

		m.editor.SetContent(content)
		m.editor.SetCursorPositionEnd()
		ed, edCmd := m.editor.Update(msg)
		m.editor = ed.(editor.Model)

		return m, tea.Batch(
			watchStreamCmd(m.respChan, m.errChan),
			edCmd,
		)

	case streamErrorMsg:
		m.loading = false
		m.loadingChunks = false
		m.error = msg.error
		return m, nil

	case streamCompleteMsg:
		m.loadingChunks = false
		currentContent := m.editor.GetCurrentContent() + "\n\n"
		m.editor.SetContent(currentContent)
		m.editor.SetCursorPosition(0, 0)
		if out, err := m.markdown.Render(currentContent); err != nil {
			m.viewport.SetContent(currentContent)
		} else {
			m.viewport.SetContent(out)
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			if m.loadingChunks {
				return m, nil
			}

			switch m.currentView {
			case reviewViewEditor:
				m.currentView = reviewViewMarkdown

			case reviewViewMarkdown:
				m.currentView = reviewViewEditor
			}
		}
	}

	var cmds []tea.Cmd

	if !m.loading {
		switch m.currentView {
		case reviewViewMarkdown:
			vp, cmd := m.viewport.Update(msg)
			m.viewport = vp
			cmds = append(cmds, cmd)

		case reviewViewEditor:
			editorModel, cmd := m.editor.Update(msg)
			m.editor = editorModel.(editor.Model)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func startStreamCmd(llm llm.LLM, ctx context.Context, prompt string) tea.Cmd {
	return func() tea.Msg {
		respChan, errChan := llm.Stream(ctx, prompt)
		return streamReadyMsg{
			respChan: respChan,
			errChan:  errChan,
		}
	}
}

func watchStreamCmd(respChan <-chan llm.Response, errChan <-chan error) tea.Cmd {
	return func() tea.Msg {
		select {
		case resp, ok := <-respChan:
			if !ok {
				return streamCompleteMsg{}
			}

			return streamChunkMsg{content: resp.Content}

		case err, ok := <-errChan:
			if ok && err != nil {
				// Check if it's a context cancellation - don't show as error
				if errors.Is(err, context.Canceled) {
					return streamCompleteMsg{}
				}
				return streamErrorMsg{error: err}
			}
			// Error channel closed without error
			return nil
		}
	}
}

func (m reviewModel) View() string {
	if m.loading {
		return lipgloss.NewStyle().Padding(2).Render(m.spinner.View() + " " + styles.Accent.Render(m.loadingMsg))
	}

	if m.error != nil {
		err := styles.Wrap(max(m.width-6, 1), styles.Error.Render("Error: "+m.error.Error()))
		return styles.Subtext0.Padding(2).Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				err,
				"\n",
				"Press r to retry",
				"Press ctrl+c to quit",
			),
		)
	}

	switch m.currentView {
	case reviewViewMarkdown:
		return m.viewport.View() + "\n" + m.statusBar()
	case reviewViewEditor:
		return m.editor.View() + "\n" + m.statusBar()
	default:
		return ""
	}
}

func (m *reviewModel) startReview(ctx context.Context) tea.Cmd {
	m.contentBuilder = &strings.Builder{}
	m.loading = true
	m.error = nil
	m.spinner.Spinner = spinner.Dot
	m.editor.SetContent("")

	return tea.Batch(
		m.spinner.Tick,
		m.dispatchLoadingMsg(),
		startStreamCmd(m.llm, ctx, m.prompt),
	)
}

func (m *reviewModel) dispatchLoadingMsg() tea.Cmd {
	return dispatchLoadingMessage(reviewLoadingMsg{message: m.getLoadingMessage()})
}

func (m *reviewModel) getLoadingMessage() string {
	return m.loadingMsgPicker.next()
}

func (m *reviewModel) statusBar() string {
	var statusBar string
	if m.loadingChunks {
		reviewer := m.reviewer.Name + " is typing..."
		statusBar = styles.Crust.Render(m.spinner.View()) + styles.Accent.Background(styles.Crust.GetBackground()).Render(" "+reviewer)
	} else {
		reviewer := m.reviewer.Name + " is done reviewing"
		statusBar = styles.Accent.Render(reviewer)
	}

	help := styles.Primary.Background(styles.Crust.GetBackground()).Render("?")
	gap := m.width - lipgloss.Width(statusBar) - lipgloss.Width(help) - 2

	statusBar += styles.Crust.Render(strings.Repeat(" ", max(0, gap)))

	return styles.Crust.Width(m.width).Padding(0, 1).Render(statusBar + help)
}

func reviewHelp(width int, forCommits bool) string {
	commands := []struct {
		Command, Description string
	}{
		{"tab", "toggle between editor view and markdown view"},
		{"c", "generate commit message (for staged changes)"},
		{"C", "generate commit message for all changes (staged, unstaged, untracked)"},
		{"?", "toggle help"},
		{"ctrl+c", "quit"},
	}

	if forCommits {
		commands = slices.Delete(commands, 1, 3)
	}

	title := styles.Text.Bold(true).Margin(1, 0, 0, 1).Render("Help")
	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		help.RenderCmdHelp(width, commands),
	)
}
