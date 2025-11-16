package tui

import (
	"context"
	"errors"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ionut-t/bark/pkg/llm"
	"github.com/ionut-t/coffee/styles"
	editor "github.com/ionut-t/goeditor/adapter-bubbletea"
)

var prLoadingMessages = [...]string{
	"Crafting PR description that's more than 'see title'...",
	"Writing something better than 'updated code'...",
	"Avoiding the dreaded 'WIP' description...",
	"Resisting 'small fix' and 'minor changes'...",
	"Actually describing what changed this time...",
	"Making your changes sound more impressive...",

	"Filling out that PR template you always skip...",
	"Adding context future you will thank you for...",
	"Ticking all the boxes literally and figuratively...",
	"Writing 'What changed' vs 'What you meant to change'...",
	"Documenting what you actually did vs what you said you'd do...",

	"Preparing to add 'screenshots later'...",
	"Generating 'before/after' you'll definitely forget to add...",
	"Remembering you should include a demo GIF...",
	"Making note to add video walkthrough (spoiler: you won't)...",

	"Preparing for inevitable 'can you add tests?'...",
	"Getting ready for 'what about edge cases?'...",
	"Anticipating 'did you consider...' comments...",
	"Pre-emptively addressing reviewer concerns...",
	"Writing description that answers questions before they're asked...",

	"Explaining why 500 lines changed in package-lock.json...",
	"Justifying that one line change that took 3 hours...",
	"Describing your refactoring journey briefly...",
	"Summarising that rabbit hole you went down...",
	"Converting your commit history into coherent narrative...",

	"Checking if this is actually breaking...",
	"Assessing blast radius of your changes...",
	"Documenting the database migrations you hope work...",
	"Noting backwards compatibility (fingers crossed)...",
	"Listing what could possibly go wrong...",

	"Making 2 weeks of work sound simple...",
	"Condensing your pain into a few paragraphs...",
	"Summarising that debugging nightmare concisely...",
	"Explaining why this took so long...",
	"Describing the journey, not just the destination...",

	"Listing tests you definitely ran...",
	"Documenting manual testing steps...",
	"Noting that you tested it on your machine...",
	"Preparing 'tested locally, works fine' statement...",
	"Adding checklist of things that should be tested...",

	"Linking to that Jira ticket somewhere...",
	"Finding related PRs from 6 months ago...",
	"Mentioning dependencies and blockers...",
	"Noting which issue this closes...",
	"Cross-referencing all the things...",

	"Making your hacky solution sound architectural...",
	"Converting 'I tried stuff until it worked' to technical prose...",
	"Framing emergency fixes as strategic improvements...",
	"Turning technical debt into 'future optimisations'...",
	"Adding professional gloss to your chaos...",

	"Preparing for the 'needs more context' comment...",
	"Getting ready for 'can you explain this part?'...",
	"Anticipating 'what's the motivation here?'...",
	"Pre-answering the obvious questions...",
	"Making it reviewer-friendly...",

	"Writing better than 'merge main into feature'...",
	"Explaining why you force-pushed 12 times...",
	"Justifying those 200+ file changes...",
	"Describing what's in those mysterious commits...",
	"Making your branch name make sense...",

	"Hoping CI passes this time...",
	"Preparing deployment notes just in case...",
	"Adding rollback instructions (better safe than sorry)...",
	"Noting environment-specific considerations...",
	"Warning about potential deployment gotchas...",

	"Writing description for PR description generator...",
	"Describing the indescribable...",
	"Meta-documenting your documentation...",
	"Making recursion make sense...",
	"Explaining the explainer...",

	"Crafting description that gets quick approval...",
	"Writing for reviewers who skim...",
	"Making it obvious you did your homework...",
	"Adding context for people who weren't in the planning meeting...",
	"Documenting for teammates across timezones...",

	"Summarising elegantly...",
	"Being comprehensive yet concise...",
	"Adding all the details...",
	"Making it scannable...",
	"Capturing the essence...",
	"Explaining clearly...",
	"Documenting thoroughly...",
}

type prInitReadyMsg struct{}

type prLoadingMsg struct {
	message string
}

type prResponseMsg struct {
	message string
	error   error
}

type prModel struct {
	editor           editor.Model
	loading          bool
	loadingMsg       string
	loadingMsgPicker *loadingMessagePicker
	spinner          spinner.Model
	llm              llm.LLM
	error            error
	prompt           string
	response         string
	showPrompt       bool
}

func newPRModel(llm llm.LLM, width, height int) prModel {
	textEditor := editor.New(width, height)
	textEditor.SetLanguage("markdown", styles.EditorLanguageTheme())
	textEditor.SetExtraHighlightedContextLines(300)
	textEditor.WithTheme(styles.EditorTheme())
	textEditor.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styles.Primary

	m := prModel{
		editor:           textEditor,
		spinner:          sp,
		loading:          true,
		llm:              llm,
		loadingMsgPicker: newLoadingMessagePicker(prLoadingMessages[:]),
	}

	m.loadingMsg = m.getLoadingMessage()
	return m
}

func (m *prModel) setPrompt(prompt string) {
	m.prompt = prompt
}

func (m *prModel) setSize(width, height int) {
	m.editor.SetSize(width, height)
}

func (m prModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *prModel) startPRDescriptionGeneration(ctx context.Context) tea.Cmd {
	m.error = nil
	m.loading = true

	return tea.Batch(
		m.spinner.Tick,
		m.dispatchLoadingMsg(),
		getPRMessage(ctx, m.llm, m.prompt),
	)
}

func (m prModel) Update(msg tea.Msg) (prModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case prLoadingMsg:
		if !m.loading {
			return m, nil
		}

		m.loadingMsg = msg.message
		return m, m.dispatchLoadingMsg()

	case prResponseMsg:
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

		m.response = msg.message
		m.editor.SetContent(msg.message + "\n\n")

	case editor.QuitMsg:
		return m, tea.Quit

	case editor.SaveMsg:
		return m, writeToDisk(&m.editor, msg.Path, msg.Content)

	case editor.SearchResultsMsg:
		if len(msg.Positions) == 0 {
			return m, DispatchNoSearchResultsError(&m.editor)
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			m.showPrompt = !m.showPrompt

			if m.showPrompt {
				m.editor.SetContent(m.prompt + "\n\n")
			} else {
				m.editor.SetContent(m.response + "\n\n")
			}
		}
	}

	ed, cmd := m.editor.Update(msg)
	m.editor = ed.(editor.Model)

	return m, cmd
}

func (m prModel) View() string {
	if m.loading {
		return lipgloss.NewStyle().Padding(2).Render(lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.spinner.View(),
			styles.Accent.Render(" "+m.loadingMsg),
		))
	}

	if m.error != nil {
		err := styles.Wrap(80, styles.Error.Render("Error: "+m.error.Error()))
		return styles.Subtext0.Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				"\n\n",
				err,
				"\n",
				"Press r to retry",
			),
		)
	}

	return m.editor.View()
}

func getPRMessage(ctx context.Context, llm llm.LLM, prompt string) tea.Cmd {
	return func() tea.Msg {
		message, err := llm.Generate(ctx, prompt)
		return prResponseMsg{
			message: message,
			error:   err,
		}
	}
}

func (m *prModel) dispatchLoadingMsg() tea.Cmd {
	return dispatchLoadingMessage(prLoadingMsg{message: m.getLoadingMessage()})
}

func (m *prModel) getLoadingMessage() string {
	return m.loadingMsgPicker.next()
}
