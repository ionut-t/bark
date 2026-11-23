package tui

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ionut-t/bark/v2/internal/check"
	"github.com/ionut-t/bark/v2/internal/debuglog"
	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/ionut-t/bark/v2/internal/prices"
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
	"Channelling the wisdom of ancient code masters...",
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
	"Mobilising the refactoring task force...",
	"Unleashing the linter army...",

	"Following the trail of code smells...",
	"Investigating suspicious patterns...",
	"Gathering evidence of anti-patterns...",
	"CSI: Code Scene Investigation...",

	"Conducting peer review experiments...",
	"Analysing under a microscope...",
	"Testing hypothesis: 'This will work'... unlikely...",
	"Peer reviewing in zero gravity...",

	"Putting the kettle on, this could take a while...",
	"Having a quick cuppa before diving in...",
	"Dunking a biscuit in your diff...",
	"Queueing politely behind the other reviews...",
	"Muttering 'well, that's not ideal' under its breath...",
	"Saying 'interesting choice' in the most British way possible...",
	"Preparing a strongly worded letter about your indentation...",
	"Tutting quietly at your error handling...",
	"Apologising in advance for what's coming...",
	"Keeping calm and carrying on reading...",

	"Finding your code a bit of a dog's dinner...",
	"Discovering it's all gone a bit pear-shaped...",
	"Wondering who left this function in such a state...",
	"Spotting a dodgy loop or two...",
	"Hoping this isn't a load of old cobblers...",
	"Concluding it's not bad, considering...",
	"Rating your code 'could be worse'...",

	"Minding the gap in your test coverage...",
	"Waiting for the review, like a bus, three at once...",
	"Sorry to bother you, but is this meant to be global...",
	"Checking whether this null pointer is having a laugh...",
	"Reading your comments with a raised eyebrow...",
	"Fetching a brolly for the incoming storm of feedback...",
	"Blaming the weather for your flaky tests...",
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

// truncated in a stream message is true once the model said it stopped at
// its output limit.
type streamChunkMsg struct {
	content   string
	usage     llm.Usage
	hasUsage  bool
	truncated bool
}

type streamCompleteMsg struct {
	usage     llm.Usage
	hasUsage  bool
	truncated bool
}

// checkDoneMsg carries the outcome of Bark's check for the review started
// as run number run.
type checkDoneMsg struct {
	run int
	// events are the review's events as the check left them, checked how
	// many findings it gave a verdict on, and unchecked how many it didn't.
	events             []findings.Event
	checked, unchecked int
	// err is set if the check failed; the review is then left unchecked.
	err error
	// usage is the check call's token usage, or nil.
	usage *llm.Usage
}

type reviewModel struct {
	width, height  int
	editor         editor.Model
	llm            llm.LLM
	respChan       <-chan llm.Response
	errChan        <-chan error
	contentBuilder *strings.Builder
	// structured means the model returns findings as JSON Lines; renderer
	// turns them into Markdown as they stream.
	structured bool
	// houseRules describes the house rules for the BARK_DEBUG log.
	houseRules string
	renderer   *findings.Renderer
	// secrets are the findings Bark made itself for secrets the diff adds
	// (see redact.Findings). A structured review shows them first; the
	// model is told not to report them.
	secrets []findings.Event
	// minSeverity leaves out less severe findings; "" shows them all.
	minSeverity findings.Severity
	// raw is the model's output so far; Bark's check parses it once the
	// review is done.
	raw *strings.Builder
	// check, if not nil, checks a finished structured review's findings.
	check *check.Review
	// checkTimeout is how long the check may take.
	checkTimeout time.Duration
	checking     bool
	// checkSummary sums up what Bark's check kept, for the status line;
	// empty until a check is done.
	checkSummary string
	// truncated is true when the model cut the review off at its output
	// limit; the review then ends with findings.TruncatedNote.
	truncated bool
	// diffNote starts the review when the diff was cut to fit its limit
	// or came with warnings; empty otherwise.
	diffNote string
	// run counts the reviews started, so the result of a check for a
	// review that was restarted is ignored.
	run int
	// debugLog is the BARK_DEBUG log of the running review, or nil.
	debugLog      *debuglog.Log
	logDir        string
	provider      string
	pricing       *prices.Pricing
	reviewer      reviewers.Reviewer
	system        string
	prompt        string
	showPrompt    bool
	response      string
	loader        loader
	loading       bool
	loadingChunks bool
	error         error
	styles        styles.Styles
	llmModel      string
}

func newReviewModel(reviewer reviewers.Reviewer, system, prompt string, width, height int, llm llm.LLM) reviewModel {
	textEditor := newEditor(width, height)
	textEditor.DisableInsertMode(true)
	textEditor.SetExtraHighlightedContextLines(streamingHighlightContextLines)
	textEditor.Focus()
	textEditor.SetMoveAcrossLines(true)

	return reviewModel{
		width:         width,
		height:        height,
		editor:        textEditor,
		llm:           llm,
		system:        system,
		prompt:        prompt,
		loading:       true,
		loadingChunks: true,
		loader:        newLoader(),
		reviewer:      reviewer,
	}
}

func (m *reviewModel) setStructured(structured bool) {
	m.structured = structured
}

// setSecrets sets the findings Bark made for the secrets the diff adds.
func (m *reviewModel) setSecrets(events []findings.Event) {
	m.secrets = events
}

func (m *reviewModel) setCheck(timeout time.Duration, r check.Review) {
	m.check = &r
	m.checkTimeout = timeout
}

// setLogInfo sets what the BARK_DEBUG log needs beyond the review itself.
func (m *reviewModel) setLogInfo(logDir, provider string) {
	m.logDir = logDir
	m.provider = provider
}

// setPricing makes the BARK_DEBUG log show the review's cost; nil shows none.
func (m *reviewModel) setPricing(p *prices.Pricing) {
	m.pricing = p
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
	m.loader.SetStyles(s)
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
	case spinner.TickMsg, loaderMessageMsg:
		if !m.loader.Owns(msg) {
			return m, nil
		}
		var cmd tea.Cmd
		m.loader, cmd = m.loader.Update(msg)
		if m.loadingChunks && !m.loading {
			m.setStreamingStatusLine()
		} else if m.checking {
			m.setCheckingStatusLine()
		}
		return m, cmd

	case streamReadyMsg:
		m.respChan = msg.respChan
		m.errChan = msg.errChan
		return m, watchStreamCmd(m.respChan, m.errChan)

	case streamChunkMsg:
		if msg.hasUsage {
			m.debugLog.Write(msg.content, &msg.usage)
		} else {
			m.debugLog.Write(msg.content, nil)
		}

		m.raw.WriteString(msg.content)
		m.truncated = m.truncated || msg.truncated
		content := msg.content
		if m.renderer != nil {
			content = m.renderer.Write(content)
		}

		// A structured review may have nothing to show yet; keep the
		// loading screen until it does.
		if content == "" && m.loading {
			return m, watchStreamCmd(m.respChan, m.errChan)
		}

		if m.loading {
			// The spinner moves to the status line while the review streams.
			m.loader.StopMessages()
			m.loader.SetSpinner(spinner.Points)
		}

		m.loading = false

		m.contentBuilder.WriteString(content)
		m.editor.SetContent(utils.NormaliseCodeFences(m.contentBuilder.String()))
		_ = m.editor.SetCursorPositionEnd()

		m.setStreamingStatusLine()

		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)

		return m, tea.Batch(
			watchStreamCmd(m.respChan, m.errChan),
			cmd,
		)

	case streamErrorMsg:
		m.loader.Stop()
		m.finishLog(msg.error)
		m.loading = false
		m.loadingChunks = false
		m.error = msg.error
		return m, nil

	case streamCompleteMsg:
		if msg.hasUsage {
			m.debugLog.Write("", &msg.usage)
		}
		m.finishLog(nil)
		if m.renderer != nil {
			m.contentBuilder.WriteString(m.renderer.Close())
			m.editor.SetContent(utils.NormaliseCodeFences(m.contentBuilder.String()))
		}
		m.truncated = m.truncated || msg.truncated
		m.loading = false
		m.loadingChunks = false
		m.editor.SetExtraHighlightedContextLines(finalHighlightContextLines)
		m.response = m.withTruncatedNote(m.editor.GetCurrentContent() + "\n\n")
		m.editor.SetContent(m.response)
		_ = m.editor.SetCursorPosition(0, 0)

		if m.structured && m.check != nil {
			if events, _ := findings.DropBelow(findings.Parse(m.raw.String()), m.minSeverity); check.Count(events) > 0 {
				// The spinner keeps going in the status line while checking.
				m.loader.StopMessages()
				m.checking = true
				m.setCheckingStatusLine()
				return m, runCheckCmd(*m.check, m.checkTimeout, m.run, events)
			}
		}
		m.loader.Stop()
		m.setDoneStatusLine()

	case checkDoneMsg:
		if !m.awaitsCheck(msg) {
			return m, nil
		}
		m.checking = false
		m.loader.Stop()
		old := m.response
		if msg.err != nil {
			m.response = strings.TrimRight(m.response, "\n") + "\n\n## Bark's check\n\n" +
				"The check failed, so the findings above are unchecked: " + msg.err.Error() + "\n"
		} else {
			// Shown again from the checked events, each finding where it
			// streamed: set-aside ones get a note under their heading, and
			// kept ones show the check's changes to them.
			// The secrets go first again; the check had nothing to doubt in
			// them, and checkSummary counts only the model's findings.
			events := append(slices.Clone(m.secrets), msg.events...)
			m.response = m.withTruncatedNote(m.withDiffNote(strings.TrimRight(utils.NormaliseCodeFences(findings.MarkedMarkdown(events)), "\n") + "\n\n"))
			m.checkSummary = checkSummary(msg.events, msg.checked, msg.unchecked)
		}
		m.setDoneStatusLine()
		if !m.showPrompt {
			pos := m.editor.GetCursorPosition()
			m.editor.SetContent(m.response)
			_ = m.editor.SetCursorPosition(followCursor(old, m.response, pos.Row), 0)
		}
		return m, nil

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
		var hasUsage, truncated bool

		for {
			select {
			case resp, ok := <-respChan:
				if !ok {
					// Flush any buffered content first; the streamChunkMsg
					// handler re-arms the watch, which then sees the closed
					// channel and completes.
					if buf.Len() > 0 {
						return streamChunkMsg{content: buf.String(), usage: usage, hasUsage: hasUsage, truncated: truncated}
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
					return streamCompleteMsg{usage: usage, hasUsage: hasUsage, truncated: truncated}
				}

				if resp.Usage != nil {
					// Copy to a value so no pointer into provider-owned
					// memory crosses into the message layer.
					usage = *resp.Usage
					hasUsage = true
				}
				truncated = truncated || resp.Truncated
				buf.WriteString(resp.Content)
				if deadline == nil {
					deadline = time.After(streamCoalesceWindow)
				}

			case <-deadline:
				return streamChunkMsg{content: buf.String(), usage: usage, hasUsage: hasUsage, truncated: truncated}

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
						return streamCompleteMsg{usage: usage, hasUsage: hasUsage, truncated: truncated}
					}
					return streamErrorMsg{error: err}
				}
			}
		}
	}
}

func (m reviewModel) View() string {
	if m.loading {
		return lipgloss.NewStyle().Padding(2).Render(m.loader.View())
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
	m.run++
	m.checking = false
	m.checkSummary = ""
	m.truncated = false
	m.raw = &strings.Builder{}
	m.contentBuilder = &strings.Builder{}
	if m.diffNote != "" {
		m.contentBuilder.WriteString(m.diffNote + "\n")
	}
	m.renderer = nil
	if m.structured {
		m.renderer = &findings.Renderer{Min: m.minSeverity}
		if len(m.secrets) > 0 {
			m.contentBuilder.WriteString(findings.Markdown(m.secrets))
		}
	}

	m.finishLog(errors.New("review restarted"))
	// A failed log must not stop the review; the TUI has nowhere to report
	// it without covering the review.
	fields := []debuglog.Field{
		{Key: "provider", Value: m.provider},
		{Key: "model", Value: m.llmModel},
		{Key: "reviewer", Value: m.reviewer.Name},
		{Key: "structured", Value: strconv.FormatBool(m.structured)},
	}
	if m.houseRules != "" {
		fields = append(fields, debuglog.Field{Key: "house_rules", Value: m.houseRules})
	}
	m.debugLog, _ = debuglog.Start(m.logDir, "review", debuglog.Meta{
		Fields:  fields,
		System:  m.system,
		Prompt:  m.prompt,
		Pricing: m.pricing,
	})
	m.loading = true
	m.error = nil
	m.editor.SetExtraHighlightedContextLines(streamingHighlightContextLines)
	m.editor.SetContent("")

	return tea.Batch(
		m.loader.Continue(loadingMessages[:]),
		startStreamCmd(m.llm, ctx, m.system, m.prompt),
	)
}

func (m *reviewModel) finishLog(err error) {
	m.debugLog.Finish(err)
	m.debugLog = nil
}

// setStreamingStatusLine bakes the current spinner frame into the editor's
// status line; call it on every spinner tick while streaming so the spinner
// animates between coalesced chunk flushes.
func (m *reviewModel) setStreamingStatusLine() {
	reviewerAction := m.styles.Accent.Background(m.styles.Surface1.GetBackground()).Render(m.reviewer.Name + " is reviewing... ")
	spacer := m.styles.Surface1.Render(" ")
	reviewerInfo := m.styles.Surface1.Render(m.loader.SpinnerView()) + spacer + reviewerAction
	m.editor.StatusLineFunc = createEditorStatusLine(m.llmModel, reviewerInfo)
}

func (m *reviewModel) setCheckingStatusLine() {
	action := m.styles.Accent.Background(m.styles.Surface1.GetBackground()).Render("Checking the findings... ")
	spacer := m.styles.Surface1.Render(" ")
	info := m.styles.Surface1.Render(m.loader.SpinnerView()) + spacer + action
	m.editor.StatusLineFunc = createEditorStatusLine(m.llmModel, info)
}

func (m *reviewModel) setDoneStatusLine() {
	reviewerInfo := m.styles.Accent.Render("Reviewed by " + m.reviewer.Name + " ")
	if m.checkSummary == "" {
		m.editor.StatusLineFunc = createEditorStatusLine(m.llmModel, reviewerInfo)
		return
	}
	m.editor.StatusLineFunc = createEditorStatusLine(m.llmModel, m.checkSummary, reviewerInfo)
}

// setDiffNotes makes the review start with a note saying how much of the
// diff it covers, if cut is not nil, and a note for each warning about
// the diff (see git.ReviewDiff.Warnings).
func (m *reviewModel) setDiffNotes(cut *git.DiffCut, warnings []string) {
	m.diffNote = ""
	if cut != nil {
		m.diffNote = findings.PartialReviewNote(cut.Summary())
	}
	for _, w := range warnings {
		m.diffNote += findings.WarningNote(w)
	}
}

// withDiffNote starts review with the note about the cut diff, if there is
// one.
func (m *reviewModel) withDiffNote(review string) string {
	if m.diffNote == "" {
		return review
	}
	return m.diffNote + "\n" + review
}

// withTruncatedNote ends review with findings.TruncatedNote if the model
// cut it off.
func (m *reviewModel) withTruncatedNote(review string) string {
	if !m.truncated {
		return review
	}
	return strings.TrimRight(review, "\n") + "\n\n" + findings.TruncatedNote + "\n"
}

// awaitsCheck reports whether msg is the result of the check the review is
// waiting for, not one for a review that was restarted.
func (m *reviewModel) awaitsCheck(msg checkDoneMsg) bool {
	return msg.run == m.run && m.checking
}

// runCheckCmd runs Bark's check on the findings of review number run,
// giving up after timeout.
func runCheckCmd(r check.Review, timeout time.Duration, run int, events []findings.Event) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		res, err := r.Run(ctx, events)
		if err != nil {
			return checkDoneMsg{run: run, usage: res.Usage, err: err}
		}
		return checkDoneMsg{run: run, usage: res.Usage, events: res.Events, checked: res.Checked, unchecked: res.Unchecked}
	}
}

// checkSummary sums up in a line how many of the checked findings Bark's
// check kept, and how many it gave no verdict on, or "" if it checked none.
func checkSummary(events []findings.Event, checked, unchecked int) string {
	s := keptSummary(events, checked)
	if s != "" && unchecked > 0 {
		s += "; " + strconv.Itoa(unchecked) + " unchecked"
	}
	return s
}

// keptSummary says how many of the checked findings Bark's check kept, or
// "" if it checked none.
func keptSummary(events []findings.Event, checked int) string {
	if checked == 0 {
		return ""
	}
	setAside := 0
	for _, e := range events {
		if e.Finding != nil && e.Finding.SetAside() {
			setAside++
		}
	}
	switch {
	case setAside == 0 && checked == 1:
		return "Bark's check kept the finding"
	case setAside == 0:
		return "Bark's check kept all " + strconv.Itoa(checked) + " findings"
	default:
		return "Bark's check kept " + strconv.Itoa(checked-setAside) + " of " + strconv.Itoa(checked) + " findings"
	}
}

// followCursor returns the row in updated that shows what row showed in old,
// where updated is old after Bark's check: the same findings in the same
// order, some with a note under their heading, a new severity or a dropped
// suggestion. Headings are matched by position, not text, as two findings
// can share one and the check can change it. Above every heading, row stays
// as it is, within updated.
func followCursor(old, updated string, row int) int {
	oldLines := strings.Split(old, "\n")
	newLines := strings.Split(updated, "\n")
	clamp := func(r int) int { return max(0, min(r, len(newLines)-1)) }

	oldHeadings, newHeadings := headings(oldLines), headings(newLines)
	k := -1
	for i, h := range oldHeadings {
		if h > row {
			break
		}
		k = i
	}
	if k < 0 || k >= len(newHeadings) {
		return clamp(row)
	}

	h, i := oldHeadings[k], newHeadings[k]
	offset := row - h
	// The note sits on the heading's second line below, ahead of everything
	// else in the finding.
	if offset >= 2 {
		offset += noteLines(newLines, i) - noteLines(oldLines, h)
	}
	if k+1 < len(newHeadings) {
		// Stay within the finding if it got shorter, on its last line
		// rather than the blank one before the next heading.
		offset = min(offset, max(0, newHeadings[k+1]-2-i))
	}
	return clamp(i + offset)
}

// headings returns the indices of the Markdown headings in lines, leaving
// out lines in code fences, such as a suggestion's "#include".
func headings(lines []string) []int {
	var out []int
	fence := ""
	for i, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case fence != "":
			if strings.HasPrefix(t, fence) && strings.Trim(t, "`") == "" {
				fence = ""
			}
		case strings.HasPrefix(t, "```"):
			fence = t[:len(t)-len(strings.TrimLeft(t, "`"))]
		case strings.HasPrefix(l, "#"):
			out = append(out, i)
		}
	}
	return out
}

// noteLines is how many lines the check's note under the heading at h takes
// up, with the blank line after it: 2, or 0 without a note.
func noteLines(lines []string, h int) int {
	if h+2 < len(lines) && (strings.HasPrefix(lines[h+2], findings.DoubtedNote) ||
		strings.HasPrefix(lines[h+2], findings.RaisedBeforeNote) || strings.HasPrefix(lines[h+2], findings.DismissedNote) ||
		strings.HasPrefix(lines[h+2], findings.UncheckedNote)) {
		return 2
	}
	return 0
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
