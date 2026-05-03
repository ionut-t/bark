package tui

import (
	"context"
	"errors"
	"slices"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ionut-t/bark/v2/internal/utils"
	"github.com/ionut-t/bark/v2/pkg/llm"
	"github.com/ionut-t/coffee/help"
	"github.com/ionut-t/coffee/styles"
	editor "github.com/ionut-t/goeditor"
)

var (
	loadingViewStyle   = lipgloss.NewStyle().Padding(2, 0, 0)
	loadingViewPadding = lipgloss.NewStyle().Padding(0, 2)
)

var generationPhaseLoadingMessages = [...]string{
	"Crafting the perfect commit message...",
	"Avoiding 'fix stuff' and 'updated files'...",
	"Resisting the urge to write 'WIP'...",
	"Trying not to write 'minor changes'...",
	"Suppressing 'fixed bug' instincts...",
	"Channeling conventional commit energy...",

	"git commit -m 'AI did it'...",
	"Blaming previous commits...",
	"Rewriting git history... wait, wrong feature...",
	"Squashing your shame into one commit...",
	"Cherry-picking the best words...",
	"Rebasing your expectations...",

	"Converting 'idk why this works' to professional speak...",
	"Translating 'YOLO' into corporate...",
	"Making 'it works now' sound intentional...",
	"Disguising trial and error as strategy...",
	"Framing luck as expertise...",

	"Deciding between fix, feat, or chore...",
	"Adding appropriate emoji... just kidding...",
	"Determining if this is breaking or not...",
	"Classifying your chaos...",

	"Analyzing what you actually changed...",
	"Figuring out what this diff means...",
	"Reading your mind (and your code)...",
	"Decoding the intent behind these changes...",
	"Understanding what future you will need to know...",

	"Taking longer than the actual code changes...",
	"Spending more time on message than code...",
	"Practicing the art of summarization...",
	"Condensing hours of work into 50 characters...",

	"Writing commit message for commit message generator...",
	"Committing to writing better commits...",
	"Meta-analyzing your changes...",
	"Generating commit inception...",

	"Checking if anyone will actually read this...",
	"Preparing for future blame annotations...",
	"Writing for your future self...",
	"Documenting for the archaeologists...",
	"Creating tomorrow's git log...",

	"Summarizing your refactoring journey...",
	"Describing technical debt payments...",
	"Explaining the unexplainable...",
	"Justifying questionable decisions...",

	"Consulting the commit message gods...",
	"Channeling Linus Torvalds...",
	"Computing semantic similarity...",
	"Training on 10 million commit messages...",
	"Avoiding passive voice...",

	"Summarizing brilliance...",
	"Capturing context...",
	"Being concise...",
	"Staying under 72 characters...",
	"Making it meaningful...",
}

var commitPhaseLoadingMessages = [...]string{
	"Sealing the deal with Git...",
	"Locking in your genius...",
	"Making it official...",
	"Putting a bow on your code...",
	"Sending your changes to the future...",
	"Committing to excellence...",
	"Locking in those improvements...",
	"Making your code history...",
	"Finalising the masterpiece...",
	"Putting your stamp on it...",
	"Making you take the blame...",
	"Sending your changes off to Git heaven...",
	"Locking in your legacy...",
	"git commit -m 'AI did it'...",
	"git reset --hard HEAD~1000 ... wait, wrong feature 👿...",
	"Deleting your changes... just kidding 😈...",
	"Squashing your shame into one commit...",
	"Committing your breaking changes...",
	"Reverting your changes 😈...",
	"rm -rf . ... 👺 oh, wrong command...",
}

type commitGenerationLoadingMsg struct {
	message string
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

	editor          editor.Model
	commitAll       bool
	loading         bool
	loadingMsg      string
	spinner         spinner.Model
	llm             llm.LLM
	prompt          string
	error           error
	response        string
	isShowingPrompt bool
	styles          styles.Styles
	isDarkMode      bool

	loadingMsgPicker    *loadingMessagePicker
	committingMsgPicker *loadingMessagePicker

	outChan   <-chan string
	errChan   <-chan error
	gitOutput []string

	viewport viewport.Model
}

type commitChangesMsg struct {
	message   string
	commitAll bool
}

type commitOutputMsg struct {
	line string
}

type commitStatusMsg struct {
	error error
}

type commitStreamStartMsg struct {
	outChan <-chan string
	errChan <-chan error
}

func newCommitChangesModel(llm llm.LLM, prompt string, commitAll bool, width, height int) commitChangesModel {
	textEditor := editor.New(width, height)
	textEditor.DisableCommandMode(true)
	textEditor.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	m := commitChangesModel{
		width:     width,
		height:    height,
		editor:    textEditor,
		spinner:   sp,
		loading:   true,
		llm:       llm,
		prompt:    prompt,
		commitAll: commitAll,
		viewport:  viewport.New(),

		loadingMsgPicker:    newLoadingMessagePicker(generationPhaseLoadingMessages[:]),
		committingMsgPicker: newLoadingMessagePicker(commitPhaseLoadingMessages[:]),
	}

	m.loadingMsg = m.getGenerationLoadingMessage()

	m.viewport.SetWidth(width - 4)
	m.viewport.SetHeight(max(5, height-5))

	return m
}

func (m *commitChangesModel) setStyles(s styles.Styles, isDarkMode bool) {
	m.styles = s
	m.isDarkMode = isDarkMode
	m.editor.WithTheme(styles.EditorTheme(s))
	m.spinner.Style = s.Primary
}

func (m *commitChangesModel) setSize(width, height int) {
	m.width = width
	m.height = height
	m.viewport.SetWidth(width - 4)
	m.viewport.SetHeight(max(5, height-5))
}

func (m commitChangesModel) Init() tea.Cmd {
	return nil
}

func (m *commitChangesModel) startCommitGeneration(ctx context.Context) tea.Cmd {
	m.error = nil
	m.loading = true

	return tea.Batch(
		m.spinner.Tick,
		m.dispatchCommitGenerationLoadingMsg(),
		getCommitMessage(ctx, m.llm, m.prompt),
	)
}

func (m commitChangesModel) Update(msg tea.Msg) (commitChangesModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case commitGenerationLoadingMsg:
		if !m.loading {
			return m, nil
		}

		m.loadingMsg = msg.message
		return m, m.dispatchCommitGenerationLoadingMsg()

	case commitLoadingMsg:
		if !m.loading {
			return m, nil
		}

		m.loadingMsg = msg.message
		return m, m.dispatchCommittingLoadingMsg()

	case commitOutputMsg:
		m.gitOutput = append(m.gitOutput, msg.line)
		m.viewport.SetContent(strings.Join(m.gitOutput, "\n"))
		m.viewport.GotoBottom()

		return m, listenCommitOutput(m.outChan, m.errChan)

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
		m.editor.SetSize(m.width-4, max(10, m.height-lipgloss.Height(m.getHeader())-1))

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
				m.editor.SetLanguage("markdown", styles.EditorLanguageTheme(m.isDarkMode))
				m.editor.SetExtraHighlightedContextLines(300)
			} else {
				m.editor.SetContent(m.response)
				m.editor.SetLanguage("", "")
			}

		case "alt+enter", "ctrl+s":
			m.loading = true
			m.loadingMsg = m.getCommittingLoadingMsg()
			return m, m.dispatch()
		}
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd

	m.editor, cmd = m.editor.Update(msg)
	cmds = append(cmds, cmd)

	if m.isShowingPrompt {
		m.prompt = m.editor.GetCurrentContent()
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m commitChangesModel) View() string {
	if m.loading {
		spinnerView := loadingViewPadding.Render(
			lipgloss.JoinHorizontal(
				lipgloss.Top,
				m.spinner.View(),
				m.styles.Accent.Render(" "+m.loadingMsg),
			),
		)

		if len(m.gitOutput) > 0 {
			return loadingViewStyle.Render(
				lipgloss.JoinVertical(
					lipgloss.Left,
					spinnerView,
					loadingViewPadding.
						Width(m.width).
						Border(lipgloss.NormalBorder(), true, false, false).
						Render(m.viewport.View()),
				),
			)
		}

		return loadingViewStyle.Render(spinnerView)
	}

	if m.error != nil {
		return lipgloss.NewStyle().Padding(2).Render(m.getError())
	}

	header := m.getHeader()
	headerHeight := lipgloss.Height(header)
	editorHeight := m.height - headerHeight - 1

	m.editor.SetSize(m.width-4, max(10, editorHeight))

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		m.editor.View(),
	)
}

func (m *commitChangesModel) getError() string {
	return m.styles.Subtext0.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			m.styles.Error.Render(styles.Wrap(80, "Error: "+m.error.Error())),
			"\n",
			lipgloss.NewStyle().
				Width(m.width-4).
				Padding(1, 0).
				Border(lipgloss.NormalBorder(), true, false, false).
				Render(m.styles.Info.Render("Press r to retry, or ctrl+c to quit.")),
		),
	)
}

func (m *commitChangesModel) getHeader() string {
	height := 7

	if m.isShowingPrompt {
		height = 6
	}

	header := lipgloss.NewStyle().Height(height).Render(m.commitChangesHelp())

	border := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderBottomForeground(m.styles.Primary.GetForeground())

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
			},
			) bool {
				return c.Command == "i" || c.Command == "ctrl+r" || c.Command == "tab"
			},
		)

		commands = slices.Insert(commands, 0, struct {
			Command     string
			Description string
		}{"esc", "exit insert mode"},
		)
	}

	return help.RenderCmdHelp(m.styles, m.width, commands)
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

func (m *commitChangesModel) dispatchCommitGenerationLoadingMsg() tea.Cmd {
	return dispatchLoadingMessage(commitGenerationLoadingMsg{message: m.getGenerationLoadingMessage()})
}

func (m *commitChangesModel) dispatchCommittingLoadingMsg() tea.Cmd {
	return dispatchLoadingMessage(commitLoadingMsg{message: m.getCommittingLoadingMsg()})
}

func (m *commitChangesModel) getGenerationLoadingMessage() string {
	return m.loadingMsgPicker.next()
}

func (m *commitChangesModel) getCommittingLoadingMsg() string {
	return m.committingMsgPicker.next()
}

func (m *commitChangesModel) canRetry() bool {
	return !m.loading && m.editor.IsNormalMode()
}

func (m *commitChangesModel) dispatch() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.dispatchCommittingLoadingMsg(),
		utils.DispatchMsg(
			commitChangesMsg{
				message:   m.editor.GetCurrentContent(),
				commitAll: m.commitAll,
			},
		),
	)
}

func listenCommitOutput(outChan <-chan string, errChan <-chan error) tea.Cmd {
	return func() tea.Msg {
		select {
		case output, ok := <-outChan:
			if !ok {
				if err, ok := <-errChan; ok && err != nil {
					return commitStatusMsg{error: err}
				}
				return commitStatusMsg{}
			}

			return commitOutputMsg{line: output}

		case err, ok := <-errChan:
			if ok && err != nil {
				return commitStatusMsg{error: err}
			}

			return commitStatusMsg{}
		}
	}
}
