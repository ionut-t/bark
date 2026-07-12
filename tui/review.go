package tui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/ionut-t/bark/v2/internal/utils"
	"github.com/ionut-t/coffee/help"
	"github.com/ionut-t/coffee/styles"
	editor "github.com/ionut-t/goeditor"
)

var loadingMessages = [...]string{
	"Brewing the perfect review...",
	"Simmering your code with a dash of criticism...",
	"Marinating your commits in wisdom...",

	"Summoning legendary reviewers...",
	"Channeling the wisdom of ancient code masters...",
	"Consulting the elder scroll of best practices...",

	"Analysing your code with ruthless precision...",
	"Preparing to unleash brutal honesty...",
	"Getting ready to roast your spaghetti code...",
	"Sharpening the critique katana...",
	"Loading the constructive criticism cannon...",
	"Calibrating the BS detector...",

	"Searching for sneaky bugs in the shadows...",
	"Counting your TODO comments... this might take a while...",
	"Deciphering what this code actually does...",
	"Untangling your logic pretzels...",
	"Archaeological expedition into your codebase...",
	"Carbon dating your legacy code...",

	"Training neural networks on your naming conventions...",
	"Computing the perfect roast-to-help ratio...",
	"Processing... complexity detected...",
	"Compiling witty remarks...",
	"Debugging your life choices...",

	"Asking Stack Overflow for the best burns...",
	"Checking if your code passes the code smell test...",
	"Measuring technical debt in Bitcoin...",
	"Running static analysis on your life choices...",
	"Peer reviewing your peer review request...",

	"Good things come to those who wait...",
	"Patience... greatness takes time...",
	"Rome wasn't debugged in a day...",
	"Quality reviews can't be rushed...",

	"Crafting feedback with Shakespearean flair...",
	"To refactor or not to refactor...",
	"Penning your code's tragic backstory...",
	"Composing a sonnet about your spaghetti code...",

	"Judging your commit messages silently...",
	"Pretending to understand your regex...",
	"Wondering why you didn't use a switch statement...",
	"Contemplating your variable names...",
	"Taking deep breaths before reading this...",
	"Preparing emotionally for what's ahead...",
	"Trying not to cry...",

	"Deploying the code critique squad...",
	"Assembling the review dream team...",
	"Mobilizing the refactoring task force...",
	"Unleashing the linter army...",

	"Following the trail of code smells...",
	"Investigating suspicious patterns...",
	"Gathering evidence of anti-patterns...",
	"CSI: Code Scene Investigation...",

	"Conducting peer review experiments...",
	"Analysing under a microscope...",
	"Testing hypothesis: 'This will work'... unlikely...",
	"Peer reviewing in zero gravity...",
}

const (
	// streamCoalesceWindow batches stream deltas into a single editor update;
	// per-delta updates re-lex the viewport and saturate the event loop.
	streamCoalesceWindow = 50 * time.Millisecond

	// Extra highlighted context lines are expensive to tokenise; keep them low
	// while streaming (cursor pinned to the bottom) and restore once the user
	// can scroll back through the finished review.
	streamingHighlightContextLines = 50
	finalHighlightContextLines     = 500
)

type streamReadyMsg struct {
	respChan  <-chan llm.Response
	errChan   <-chan error
	startedAt time.Time
}

type streamErrorMsg struct {
	error error
}

type streamChunkMsg struct {
	content  string
	usage    llm.Usage
	hasUsage bool
}

type streamCompleteMsg struct {
	usage    llm.Usage
	hasUsage bool
}

type reviewLoadingMsg struct {
	message string
}

type reviewModel struct {
	width, height    int
	editor           editor.Model
	llm              llm.LLM
	respChan         <-chan llm.Response
	errChan          <-chan error
	contentBuilder   *strings.Builder
	reviewer         reviewers.Reviewer
	system           string
	prompt           string
	showPrompt       bool
	response         string
	spinner          spinner.Model
	loading          bool
	loadingChunks    bool
	loadingMsg       string
	loadingMsgPicker *loadingMessagePicker
	error            error
	styles           styles.Styles
	llmModel         string
}

func newReviewModel(reviewer reviewers.Reviewer, system, prompt string, width, height int, llm llm.LLM) reviewModel {
	textEditor := editor.New(width, height)
	textEditor.DisableInsertMode(true)
	textEditor.SetExtraHighlightedContextLines(streamingHighlightContextLines)
	textEditor.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	m := reviewModel{
		width:            width,
		height:           height,
		editor:           textEditor,
		llm:              llm,
		system:           system,
		prompt:           prompt,
		loading:          true,
		loadingChunks:    true,
		spinner:          sp,
		reviewer:         reviewer,
		loadingMsgPicker: newLoadingMessagePicker(loadingMessages[:]),
	}

	m.loadingMsg = m.getLoadingMessage()

	return m
}

func (m *reviewModel) setUsedModel(model string) {
	m.llmModel = model
}

func (m *reviewModel) showRelativeLineNumbers(enabled bool) {
	m.editor.ShowRelativeLineNumbers(enabled)
}

func (m *reviewModel) setStyles(s styles.Styles, isDarkMode bool) {
	m.styles = s

	m.editor.WithTheme(styles.EditorTheme(s))
	m.editor.SetLanguage("markdown", styles.EditorLanguageTheme(isDarkMode))
	m.spinner.Style = s.Primary
}

func (m *reviewModel) setSize(width, height int) {
	m.width = width
	m.height = height

	m.editor.SetSize(width, height)
}

func (m reviewModel) Init() tea.Cmd {
	return nil
}

func (m reviewModel) Update(msg tea.Msg) (reviewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.loadingChunks && !m.loading {
			m.setStreamingStatusLine()
		}
		return m, cmd

	case reviewLoadingMsg:
		if !m.loading {
			return m, nil
		}

		m.loadingMsg = msg.message
		return m, m.dispatchLoadingMsg()

	case streamReadyMsg:
		m.respChan = msg.respChan
		m.errChan = msg.errChan
		return m, watchStreamCmd(m.respChan, m.errChan)

	case streamChunkMsg:
		if m.loading {
			m.spinner.Spinner = spinner.Points
		}

		m.loading = false

		m.contentBuilder.WriteString(msg.content)
		content := utils.NormaliseCodeFences(m.contentBuilder.String())

		m.editor.SetContent(content)
		_ = m.editor.SetCursorPositionEnd()

		m.setStreamingStatusLine()

		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)

		return m, tea.Batch(
			watchStreamCmd(m.respChan, m.errChan),
			cmd,
		)

	case streamErrorMsg:
		m.loading = false
		m.loadingChunks = false
		m.error = msg.error
		return m, nil

	case streamCompleteMsg:
		m.loadingChunks = false
		m.editor.SetExtraHighlightedContextLines(finalHighlightContextLines)
		m.response = m.editor.GetCurrentContent() + "\n\n"
		m.editor.SetContent(m.response)
		reviewerInfo := m.styles.Accent.Render("Reviewed by " + m.reviewer.Name + " ")
		m.editor.StatusLineFunc = createEditorStatusLine(m.llmModel, reviewerInfo)
		_ = m.editor.SetCursorPosition(0, 0)

	case editor.QuitMsg:
		return m, tea.Quit

	case editor.SaveMsg:
		return m, writeToDisk(&m.editor, msg.Path, msg.Content)

	case editor.SearchResultsMsg:
		if len(msg.Positions) == 0 {
			return m, dispatchNoSearchResultsError(&m.editor)
		}

	case configErrMsg:
		return m, m.editor.DispatchError(msg, 2*time.Second)

	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			if m.loadingChunks {
				return m, nil
			}

			m.showPrompt = !m.showPrompt
			if m.showPrompt {
				m.editor.SetContent(promptPreview(m.system, utils.NormaliseCodeFences(m.prompt)))
			} else {
				m.editor.SetContent(m.response)
			}
		}
	}

	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)

	return m, cmd
}

func startStreamCmd(llm llm.LLM, ctx context.Context, system, prompt string) tea.Cmd {
	return func() tea.Msg {
		startedAt := time.Now()
		respChan, errChan := llm.Stream(ctx, system, prompt)
		return streamReadyMsg{
			respChan:  respChan,
			errChan:   errChan,
			startedAt: startedAt,
		}
	}
}

func watchStreamCmd(respChan <-chan llm.Response, errChan <-chan error) tea.Cmd {
	return func() tea.Msg {
		var buf strings.Builder
		var deadline <-chan time.Time
		var usage llm.Usage
		var hasUsage bool

		for {
			select {
			case resp, ok := <-respChan:
				if !ok {
					// Flush any buffered content first; the streamChunkMsg
					// handler re-arms the watch, which then sees the closed
					// channel and completes.
					if buf.Len() > 0 {
						return streamChunkMsg{content: buf.String(), usage: usage, hasUsage: hasUsage}
					}
					// Providers buffer a late error before closing both
					// channels, and select picks randomly between ready
					// cases, so check for a pending error before reporting
					// the stream as complete.
					select {
					case err, ok := <-errChan:
						if ok && err != nil && !errors.Is(err, context.Canceled) {
							return streamErrorMsg{error: err}
						}
					default:
					}
					return streamCompleteMsg{usage: usage, hasUsage: hasUsage}
				}

				if resp.Usage != nil {
					// Copy to a value so no pointer into provider-owned
					// memory crosses into the message layer.
					usage = *resp.Usage
					hasUsage = true
				}
				buf.WriteString(resp.Content)
				if deadline == nil {
					deadline = time.After(streamCoalesceWindow)
				}

			case <-deadline:
				return streamChunkMsg{content: buf.String(), usage: usage, hasUsage: hasUsage}

			case err, ok := <-errChan:
				if !ok {
					// Closed without error. The stream may still have
					// content pending, so disable this case (a nil channel
					// never fires) and keep draining respChan.
					errChan = nil
					continue
				}
				if err != nil {
					// Context cancellation is not an error to surface
					if errors.Is(err, context.Canceled) {
						return streamCompleteMsg{usage: usage, hasUsage: hasUsage}
					}
					return streamErrorMsg{error: err}
				}
			}
		}
	}
}

func (m reviewModel) View() string {
	if m.loading {
		return lipgloss.NewStyle().Padding(2).Render(m.spinner.View() + " " + m.styles.Accent.Render(m.loadingMsg))
	}

	if m.error != nil {
		err := styles.Wrap(max(m.width-6, 1), m.styles.Error.Render("Error: "+m.error.Error()))
		return m.styles.Subtext0.Padding(2).Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				err,
				"\n",
				"Press r to retry",
				"Press ctrl+c to quit",
			),
		)
	}

	return m.editor.View()
}

func (m *reviewModel) startReview(ctx context.Context) tea.Cmd {
	m.contentBuilder = &strings.Builder{}
	m.loading = true
	m.error = nil
	m.spinner.Spinner = spinner.Dot
	m.editor.SetExtraHighlightedContextLines(streamingHighlightContextLines)
	m.editor.SetContent("")

	return tea.Batch(
		m.spinner.Tick,
		m.dispatchLoadingMsg(),
		startStreamCmd(m.llm, ctx, m.system, m.prompt),
	)
}

// setStreamingStatusLine bakes the current spinner frame into the editor's
// status line; call it on every spinner tick while streaming so the spinner
// animates between coalesced chunk flushes.
func (m *reviewModel) setStreamingStatusLine() {
	reviewerAction := m.styles.Accent.Background(m.styles.Surface1.GetBackground()).Render(m.reviewer.Name + " is reviewing... ")
	spacer := m.styles.Surface1.Render(" ")
	reviewerInfo := m.styles.Surface1.Render(m.spinner.View()) + spacer + reviewerAction
	m.editor.StatusLineFunc = createEditorStatusLine(m.llmModel, reviewerInfo)
}

func (m *reviewModel) dispatchLoadingMsg() tea.Cmd {
	return dispatchLoadingMessage(reviewLoadingMsg{message: m.getLoadingMessage()})
}

func (m *reviewModel) getLoadingMessage() string {
	return m.loadingMsgPicker.next()
}

func reviewHelp(width int, forCommits bool, s styles.Styles) string {
	commands := []struct {
		Command, Description string
	}{
		{"tab", "toggle between review and prompt"},
		{"c", "generate commit message (for staged changes)"},
		{"C", "generate commit message for all changes (staged and unstaged)"},
		{"ctrl+t", "show LLM usage stats"},
		{"esc", "close help"},
		{"ctrl+c", "quit"},
	}

	if forCommits {
		commands = slices.Delete(commands, 1, 3)
	}

	title := s.Text.Bold(true).Margin(1, 0, 0, 1).Render("Help")
	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		help.RenderCmdHelp(s, width, commands),
	)
}

func (m *reviewModel) canGenerateCommitMessage() bool {
	return !m.editor.IsSearchMode()
}
