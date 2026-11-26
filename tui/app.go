package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ionut-t/bark/v2/internal/check"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/ionut-t/bark/v2/internal/houserules"
	"github.com/ionut-t/bark/v2/internal/instructions"
	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/ionut-t/bark/v2/internal/llm/llm_factory"
	"github.com/ionut-t/bark/v2/internal/paths"
	"github.com/ionut-t/bark/v2/internal/prices"
	"github.com/ionut-t/bark/v2/internal/prompt"
	"github.com/ionut-t/bark/v2/internal/redact"
	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/ionut-t/bark/v2/internal/utils"
	"github.com/ionut-t/coffee/styles"
	editor "github.com/ionut-t/goeditor"
)

const (
	defaultCommitLimit = 25
	// defaultLLMTimeout is how long a model call may take when
	// llm_timeout isn't set.
	defaultLLMTimeout = 5 * time.Minute
	gitTimeout        = 30 * time.Second
)

type view int

const (
	viewInit view = iota
	viewTasks
	viewReviewOptions
	viewCommits
	viewReviewers
	viewInstructions
	viewReview
	viewCommitChanges
	viewPRDescription
	viewBranchInput
	viewPRNumberInput
	viewPRDescriptionOptions
)

type Model struct {
	width, height int

	error     error
	commitErr error

	currentView view

	llm    llm.LLM
	config config.Config
	paths  paths.Paths

	tasks          tasksModel
	selectedTask   Task
	individualTask bool

	commits        commitsModel
	selectedCommit *git.Commit
	selectCommit   bool

	stagedOnly bool

	reviewOptions        reviewOptionsModel
	selectedReviewOption ReviewOption
	reviewers            reviewersModel
	selectedReviewer     *reviewers.Reviewer
	review               reviewModel
	reviewerName         string

	instructions        instructionsModel
	selectedInstruction string
	instructionValues   []string
	skipInstruction     bool

	commitChanges commitChangesModel

	branch      string
	branchErr   error
	branchInput branchInputModel

	prNumber          string
	withPRDescription bool
	prNumberInput     prNumberInputModel

	prDescriptionOptions prDescriptionOptionsModel

	pr prModel

	hint string

	showHelp bool
	message  string

	// preparing is set while the diff and instructions load for a review,
	// commit message or PR description, before the LLM call starts. The
	// loader shows over the current view meanwhile, then moves to the
	// screen it was loading.
	preparing bool
	loader    loader

	reviewCancelFunc    context.CancelFunc
	operationCancelFunc context.CancelFunc

	styles     styles.Styles
	isDarkMode bool
	pendingCmd tea.Cmd

	llmProvider string
	// pricing prices the calls, or is nil without a prices file listing
	// the model.
	pricing      *prices.Pricing
	lastUsage    *usageStats
	genStartedAt time.Time
	showStats    bool

	viewport viewport.Model
}

type Options struct {
	// Warnings, if not nil, gets the warnings about secrets masked before a
	// call; the TUI has no room for them while it runs (see redact.Wrap).
	Warnings          io.Writer
	Paths             paths.Paths
	ReviewerName      string
	Instructions      []string
	Branch            string
	PR                string
	SelectCommit      bool
	Config            config.Config
	StagedOnly        bool
	SkipInstruction   bool
	Task              Task
	ReviewOption      ReviewOption
	Hint              string
	WithPRDescription bool
}

func New(options Options) *Model {
	// Dark until the terminal answers Init's request: see Model.Init.
	isDarkMode := true

	llm, provider, err := llm_factory.New(context.Background(), options.Config)
	if err != nil {
		return &Model{
			error: err,
		}
	}
	llm = redact.Wrap(llm, options.Warnings)

	// The TUI has nowhere to warn about a prices file it can't use; it just
	// shows no cost.
	model, _ := options.Config.GetLLMModel()
	pricing, _ := prices.For(options.Paths.Cache, provider, model)

	if !git.IsGitRepo() {
		return &Model{
			error: git.ErrNotAGitRepository,
		}
	}

	currentView := viewInit
	if options.Task == TaskNone {
		currentView = viewTasks
	}

	styles := styles.New(isDarkMode)

	var pendingCmd tea.Cmd
	if options.Task != TaskNone {
		pendingCmd = utils.DispatchMsg(taskSelectedMsg{task: options.Task})
	}

	m := &Model{
		width:                80,
		height:               24,
		currentView:          currentView,
		llm:                  llm,
		llmProvider:          provider,
		pricing:              pricing,
		config:               options.Config,
		paths:                options.Paths,
		selectCommit:         options.SelectCommit,
		reviewerName:         options.ReviewerName,
		instructionValues:    options.Instructions,
		branch:               options.Branch,
		branchInput:          newBranchInputModel(options.Branch),
		prNumber:             options.PR,
		withPRDescription:    options.WithPRDescription,
		prNumberInput:        newPRNumberInputModel(options.PR),
		stagedOnly:           options.StagedOnly,
		skipInstruction:      options.SkipInstruction,
		tasks:                newTasksModel(styles, isDarkMode),
		selectedTask:         options.Task,
		reviewOptions:        newReviewOptionsModel(styles, isDarkMode),
		selectedReviewOption: options.ReviewOption,
		individualTask:       options.Task != TaskNone,
		hint:                 options.Hint,
		pendingCmd:           pendingCmd,
		isDarkMode:           isDarkMode,
		styles:               styles,
		viewport:             viewport.New(),
	}

	m.branchInput.setStyles(styles)
	m.prNumberInput.setStyles(styles)
	m.prDescriptionOptions = newPRDescriptionOptionsModel(styles, isDarkMode)
	m.tasks.setStyles(styles, isDarkMode)
	m.reviewOptions.setStyles(styles, isDarkMode)

	return m
}

// sizeLists fits every built list to the window, not only the one on screen,
// so a list you go back to doesn't keep the size it had before a resize.
// Lists not built yet are zero values and get sized when they are built.
func (m *Model) sizeLists() {
	for _, l := range []*list.Model{
		&m.tasks.list,
		&m.reviewOptions.list,
		&m.prDescriptionOptions.list,
		&m.commits.list,
		&m.reviewers.list,
		&m.instructions.list,
	} {
		if len(l.Items()) > 0 {
			sizeList(l, m.width, m.height)
		}
	}
}

// llmTimeout is how long one model call may take (llm_timeout).
func (m *Model) llmTimeout() time.Duration {
	if m.config == nil {
		return defaultLLMTimeout
	}
	return m.config.GetLLMTimeout(defaultLLMTimeout)
}

func (m *Model) recordUsage(usage llm.Usage, duration time.Duration) {
	m.lastUsage = &usageStats{
		usage:    usage,
		provider: m.llmProvider,
		model:    m.getLlmModelName(),
		duration: duration,
		pricing:  m.pricing,
	}
}

// recordCheckUsage adds the usage of the check that followed the last
// review to its stats, unless the check is for a review that was restarted.
func (m *Model) recordCheckUsage(msg checkDoneMsg) {
	if msg.usage == nil || m.lastUsage == nil || !m.review.awaitsCheck(msg) {
		return
	}
	u := *msg.usage
	m.lastUsage.check = &u
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, m.pendingCmd)
}

// setDarkMode restyles the app for the terminal's background. The answer
// usually arrives before the first key press, but a task given on the command
// line may already have built its models by then.
func (m *Model) setDarkMode(isDarkMode bool) {
	if isDarkMode == m.isDarkMode {
		return
	}

	m.isDarkMode = isDarkMode
	m.styles = styles.New(isDarkMode)

	m.branchInput.setStyles(m.styles)
	m.prNumberInput.setStyles(m.styles)

	// Models built on demand are zero values until then; only a built list
	// has items and only a built editor model has its LLM client.
	for _, l := range []*list.Model{
		&m.tasks.list,
		&m.reviewOptions.list,
		&m.prDescriptionOptions.list,
		&m.reviewers.list,
		&m.instructions.list,
	} {
		if len(l.Items()) > 0 {
			restyleList(l, m.styles, isDarkMode)
		}
	}
	if len(m.commits.list.Items()) > 0 {
		m.commits.setStyles(m.styles, isDarkMode)
	}

	m.loader.SetStyles(m.styles)
	if m.review.llm != nil {
		m.review.setStyles(m.styles, isDarkMode)
	}
	if m.commitChanges.llm != nil {
		m.commitChanges.setStyles(m.styles, isDarkMode)
	}
	if m.pr.llm != nil {
		m.pr.setStyles(m.styles, isDarkMode)
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case streamReadyMsg:
		m.genStartedAt = msg.startedAt
	case streamChunkMsg:
		if msg.hasUsage {
			m.recordUsage(msg.usage, time.Since(m.genStartedAt))
		}
	case streamCompleteMsg:
		if msg.hasUsage {
			m.recordUsage(msg.usage, time.Since(m.genStartedAt))
		}
	case commitResponseMsg:
		if msg.hasUsage {
			m.recordUsage(msg.usage, msg.duration)
		}
	case prResponseMsg:
		if msg.hasUsage {
			m.recordUsage(msg.usage, msg.duration)
		}
	case checkDoneMsg:
		// Before the review model takes the message and stops waiting
		// for it.
		m.recordCheckUsage(msg)
	}

	switch msg := msg.(type) {
	case spinner.TickMsg, loaderMessageMsg:
		// Other loaders' messages go on to the screens below.
		if m.preparing && m.loader.Owns(msg) {
			var cmd tea.Cmd
			m.loader, cmd = m.loader.Update(msg)
			return m, cmd
		}

	case tea.BackgroundColorMsg:
		m.setDarkMode(msg.IsDark())
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.SetWidth(max(msg.Width-2, 10))
		m.viewport.SetHeight(max(msg.Height-10, 5))

		m.sizeLists()

		switch m.currentView {
		case viewReview:
			m.review.setSize(m.width, m.height)
		case viewCommitChanges:
			m.commitChanges.setSize(m.width, m.height)
		case viewPRDescription:
			m.pr.setSize(m.width, m.height)
		}

	case editor.RelativeNumbersChangeMsg:
		return m, setRelativeNumberCmd(m.config, msg.Enabled)

	case taskSelectedMsg:
		return m.handleSelectedTask(msg.task)

	case reviewOptionSelectedMsg:
		return m.handleSelectedReviewOption(msg.option)

	case branchSelectedMsg:
		m.branch = msg.branch
		return m, utils.DispatchMsg(listReviewersMsg{})

	case listCommitsMsg:
		return m, loadCommitsCmd(defaultCommitLimit)

	case commitsLoadedMsg:
		if msg.err != nil {
			m.error = msg.err
			return m, nil
		}

		m.commits = newCommitsModel(msg.commits)
		m.commits.setSize(m.width, m.height)
		m.commits.setStyles(m.styles, m.isDarkMode)
		m.currentView = viewCommits

	case commitSelectedMsg:
		m.selectedCommit = &msg.commit
		return m, utils.DispatchMsg(listReviewersMsg{})

	case listReviewersMsg:
		return m, loadReviewersCmd(m.paths.Config)

	case reviewersLoadedMsg:
		listReviewers := msg.reviewers
		if msg.reviewersErr != nil {
			m.error = msg.reviewersErr
		}
		m.reviewers = newReviewersModel(listReviewers, m.styles, m.isDarkMode)
		m.reviewers.setSize(m.width, m.height)

		if m.reviewerName != "" {
			if reviewer, err := reviewers.Find(m.reviewerName, listReviewers); err == nil {
				m.selectedReviewer = reviewer
				return m, utils.DispatchMsg(reviewerSelectedMsg{Reviewer: reviewer})
			}
		}

		if msg.fileErr == nil {
			return m, utils.DispatchMsg(reviewerSelectedMsg{Reviewer: msg.fileReviewer})
		} else if !errors.Is(msg.fileErr, os.ErrNotExist) {
			m.error = msg.fileErr
			return m, nil
		}

		m.currentView = viewReviewers

	case reviewerSelectedMsg:
		return m.handleSelectedReviewer(msg.Reviewer)

	case reviewInstructionsLoadedMsg:
		listInstructions := msg.instructions

		if len(m.instructionValues) > 0 {
			instruction, err := instructions.Resolve(m.instructionValues, listInstructions)
			if err != nil {
				m.error = err
				return m, nil
			}
			m.selectedInstruction = instruction
			return m, utils.DispatchMsg(instructionSelectedMsg{instruction: instruction})
		}

		if msg.overrideErr != nil {
			m.error = msg.overrideErr
			return m, nil
		} else if msg.override != "" {
			return m, utils.DispatchMsg(instructionSelectedMsg{instruction: msg.override})
		}

		if msg.instructionsErr != nil {
			m.error = msg.instructionsErr
		}

		if len(listInstructions) == 0 {
			return m, utils.DispatchMsg(instructionSelectedMsg{instruction: ""})
		}

		m.instructions = newInstructionsModel(listInstructions, m.styles, m.isDarkMode)
		m.instructions.setSize(m.width, m.height)
		m.currentView = viewInstructions

	case instructionSelectedMsg:
		return m.handleSelectedInstruction(msg.instruction)

	case reviewDiffLoadedMsg:
		return m.handleReviewDiffLoaded(msg)

	case commitDataLoadedMsg:
		return m.handleCommitDataLoaded(msg)

	case prDataLoadedMsg:
		return m.handlePRDataLoaded(msg)

	case commitChangesMsg:
		// Clean up the commit context since operation completed
		if m.operationCancelFunc != nil {
			m.operationCancelFunc()
			m.operationCancelFunc = nil
		}

		m.commitErr = nil
		m.commitChanges.loading = true
		m.commitChanges.gitOutput = nil

		return m, performCommit(msg.message, msg.commitAll)

	case commitStreamStartMsg:
		m.commitChanges.outChan = msg.outChan
		m.commitChanges.errChan = msg.errChan
		m.viewport.GotoTop()
		return m, listenCommitOutput(msg.outChan, msg.errChan)

	case commitStatusMsg:
		m.commitChanges.loading = false
		m.commitChanges.loader.Stop()

		if msg.error != nil {
			m.commitErr = msg.error
			m.viewport.SetContent(m.styles.Error.Padding(0, 2).Render(msg.error.Error()))
		} else {
			return m, tea.Quit
		}

	case prDescriptionOptionSelectedMsg:
		if msg.useGitHubPR {
			m.currentView = viewPRNumberInput
		} else {
			return m.handlePRDescription()
		}

	case cancelPRDescriptionOptionsMsg:
		if m.individualTask {
			break
		}
		m.currentView = viewTasks

	case cancelReviewOptionSelectionMsg:
		if m.individualTask {
			break
		}

		m.currentView = viewTasks
		m.selectedReviewOption = ReviewOptionNone
		// The commit task reads it too
		m.stagedOnly = false

	case prNumberSelectedMsg:
		m.prNumber = msg.prNumber
		if m.selectedTask == TaskPRDescription {
			return m.handlePRDescription()
		}
		return m, utils.DispatchMsg(listReviewersMsg{})

	case cancelPRNumberSelectionMsg:
		m.prNumber = ""
		if m.selectedTask == TaskPRDescription {
			m.currentView = viewPRDescriptionOptions
		} else {
			m.currentView = viewReviewOptions
		}

	case cancelBranchSelectionMsg:
		m.currentView = viewReviewOptions
		m.branch = ""

	case cancelReviewerSelectionMsg:
		m.selectedReviewer = nil

		if m.selectCommit {
			m.currentView = viewCommits
		} else if m.branch != "" {
			m.currentView = viewBranchInput
		} else if m.selectedReviewOption == ReviewPR {
			m.currentView = viewPRNumberInput
		} else {
			m.currentView = viewReviewOptions
		}

	case cancelCommitSelectionMsg:
		m.currentView = viewReviewOptions
		m.selectedCommit = nil

	case cancelInstructionSelectionMsg:
		m.currentView = viewReviewers
		m.selectedInstruction = ""

	case tea.KeyMsg:
		if m.preparing && msg.String() != "ctrl+c" {
			return m, nil
		}

		if m.showStats {
			switch msg.String() {
			case "esc", "ctrl+t":
				m.showStats = false
				return m, nil
			case "ctrl+c":
				// allow ctrl+c to fall through and quit
			default:
				return m, nil
			}
		}

		switch msg.String() {
		case "ctrl+c":
			if m.reviewCancelFunc != nil {
				m.reviewCancelFunc()
			}

			if m.operationCancelFunc != nil {
				m.operationCancelFunc()
			}

			return m, tea.Quit

		case "f1":
			m.showHelp = !m.showHelp

		case "ctrl+t":
			m.showStats = !m.showStats
			return m, nil

		case "esc":
			m.showHelp = false

			if m.selectedTask != TaskCommit {
				m.message = ""
			}

			if m.branchErr != nil {
				m.currentView = viewBranchInput
				m.branchErr = nil
				return m, nil
			}

			if m.commitErr != nil {
				m.commitErr = nil
			}

		case "ctrl+g":
			switch m.currentView {
			case viewCommitChanges:
				if m.commitChanges.canRetry() {
					return m.handleCommitMessageRetry()
				}
			}

		case "r":
			switch m.currentView {
			case viewReview:
				if m.review.error != nil {
					ctx, cancel := context.WithTimeout(context.Background(), m.llmTimeout())

					if m.reviewCancelFunc != nil {
						m.reviewCancelFunc()
					}

					m.reviewCancelFunc = cancel

					return m, m.review.startReview(ctx)
				}

			case viewCommitChanges:
				if m.commitChanges.error != nil {
					return m.handleCommitMessageRetry()
				}

				if m.commitErr != nil {
					return m, m.commitChanges.dispatch()
				}

			case viewPRDescription:
				if m.pr.error != nil {
					ctx, cancel := context.WithTimeout(context.Background(), m.llmTimeout())

					if m.operationCancelFunc != nil {
						m.operationCancelFunc()
					}
					m.operationCancelFunc = cancel

					return m, m.pr.startPRDescriptionGeneration(ctx)
				}
			}

		case "c", "C":
			m.message = ""
			m.error = nil

			if (m.selectedTask == TaskCommit && m.currentView != viewCommitChanges) ||
				(m.currentView == viewReview && !m.selectCommit && m.review.canGenerateCommitMessage()) {
				return m.handleCommitMessage(msg.String() == "C")
			}
		}
	}

	var cmd tea.Cmd

	switch m.currentView {
	case viewTasks:
		m.tasks, cmd = m.tasks.Update(msg)

	case viewReviewOptions:
		m.reviewOptions, cmd = m.reviewOptions.Update(msg)

	case viewCommits:
		m.commits, cmd = m.commits.Update(msg)

	case viewReviewers:
		m.reviewers, cmd = m.reviewers.Update(msg)

	case viewInstructions:
		m.instructions, cmd = m.instructions.Update(msg)

	case viewReview:
		m.review, cmd = m.review.Update(msg)

	case viewCommitChanges:
		m.commitChanges, cmd = m.commitChanges.Update(msg)

	case viewPRDescription:
		m.pr, cmd = m.pr.Update(msg)

	case viewBranchInput:
		m.branchInput, cmd = m.branchInput.Update(msg)

	case viewPRNumberInput:
		m.prNumberInput, cmd = m.prNumberInput.Update(msg)

	case viewPRDescriptionOptions:
		m.prDescriptionOptions, cmd = m.prDescriptionOptions.Update(msg)
	}

	if m.commitErr != nil {
		m.viewport, cmd = m.viewport.Update(msg)
	}

	return m, cmd
}

func (m Model) View() tea.View {
	content := m.createView()
	if m.showStats {
		content = overlayCenter(content, renderUsageStats(m.lastUsage, m.styles), m.width, m.height)
	}

	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = m.getTitle()

	return view
}

func (m Model) getTitle() string {
	title := "Bark AI"
	switch m.selectedTask {
	case TaskReview:
		title += " - Code Review"
	case TaskCommit:
		title += " - Commit Changes"
	case TaskPRDescription:
		title += " - PR Description"
	}

	return title
}

func (m Model) createView() string {
	if m.error != nil {
		if errors.Is(m.error, git.ErrNoCommitsInRepository) {
			return m.styles.Info.Padding(2).Render(
				lipgloss.JoinVertical(
					lipgloss.Left,
					"Could not find any commits.",
					"This can happen when there are no commits in the repository.",
					"Review the uncommitted changes with `bark review --changes`, or only the staged ones with `bark review --staged`.",
					"\n",
					"Press `ctrl+c` to exit.",
				),
			)
		}

		return m.styles.Error.Padding(2).Render("Error: " + m.error.Error() + "\n" + "Press `ctrl+c` to exit.")
	}

	if m.commitErr != nil {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			lipgloss.NewStyle().Margin(1, 0).Padding(0, 2).Render("Error committing changes: "),
			lipgloss.NewStyle().
				Width(m.width).
				Border(lipgloss.NormalBorder(), true, false).
				Render(m.viewport.View()),
			m.styles.Info.
				Margin(1, 2).
				Render("Press r to retry committing, esc to go back or ctrl+c to exit."),
		)
	}

	if m.message != "" {
		return m.message
	}

	if m.preparing {
		return lipgloss.NewStyle().Padding(2).Render(
			m.loader.View(),
		)
	}

	switch m.currentView {
	case viewTasks:
		return m.tasks.View()

	case viewCommits:
		return m.commits.View()

	case viewReviewers:
		return m.reviewers.View()

	case viewReviewOptions:
		return m.reviewOptions.View()

	case viewInstructions:
		return m.instructions.View()

	case viewReview:
		if m.showHelp {
			return reviewHelp(m.width, m.selectCommit, m.styles)
		}

		return m.review.View()

	case viewCommitChanges:
		return m.commitChanges.View()

	case viewPRDescription:
		return m.pr.View()

	case viewBranchInput:
		return m.branchInput.View()

	case viewPRNumberInput:
		return m.prNumberInput.View()

	case viewPRDescriptionOptions:
		return m.prDescriptionOptions.View()

	default:
		return ""
	}
}

func (m *Model) handleSelectedTask(task Task) (tea.Model, tea.Cmd) {
	m.selectedTask = task

	switch m.selectedTask {
	case TaskReview:
		if m.selectedReviewOption != ReviewOptionNone {
			return m, utils.DispatchMsg(reviewOptionSelectedMsg{option: m.selectedReviewOption})
		}

		m.currentView = viewReviewOptions
	case TaskCommit:
		return m.handleCommitMessage(!m.stagedOnly)

	case TaskPRDescription:
		if m.prNumber != "" || m.branch != "" {
			return m.handlePRDescription()
		}
		m.currentView = viewPRDescriptionOptions
	}

	return m, nil
}

func (m *Model) handleSelectedReviewer(reviewer *reviewers.Reviewer) (tea.Model, tea.Cmd) {
	m.selectedReviewer = reviewer

	if m.skipInstruction {
		return m, utils.DispatchMsg(instructionSelectedMsg{instruction: ""})
	}

	return m, loadReviewInstructionsCmd(m.paths.Config)
}

func (m *Model) handleSelectedReviewOption(option ReviewOption) (tea.Model, tea.Cmd) {
	m.selectedReviewOption = option
	// Set from the option, not only turned on: after going back from the
	// commit list, another option must not review commit "".
	m.selectCommit = option == ReviewOptionCommit
	m.stagedOnly = option == ReviewOptionStagedChanges

	switch m.selectedReviewOption {
	case ReviewOptionCurrentChanges, ReviewOptionStagedChanges:
		return m, utils.DispatchMsg(listReviewersMsg{})
	case ReviewOptionCommit:
		return m, utils.DispatchMsg(listCommitsMsg{})
	case ReviewOptionBranch:
		m.currentView = viewBranchInput
		if m.branch != "" {
			return m, utils.DispatchMsg(listReviewersMsg{})
		}
	case ReviewPR:
		if m.prNumber != "" {
			return m, utils.DispatchMsg(listReviewersMsg{})
		}
		m.currentView = viewPRNumberInput
	}

	return m, nil
}

func (m *Model) handleSelectedInstruction(instruction string) (tea.Model, tea.Cmd) {
	var commitHash string
	if m.selectCommit && m.selectedCommit != nil {
		commitHash = m.selectedCommit.Hash
	}

	return m, tea.Batch(m.startPreparing(loadingMessages[:]), loadReviewDiffCmd(
		reviewDiffCmdParams{
			prNumber:          m.prNumber,
			branch:            m.branch,
			diffLimit:         git.LimitFrom(m.config),
			selectCommit:      m.selectCommit,
			commitHash:        commitHash,
			stagedOnly:        m.stagedOnly,
			instruction:       instruction,
			withPRDescription: m.withPRDescription,
			contextEnrichment: m.config.GetContextEnrichment(),
			houseRules:        m.config.GetHouseRules(),
			houseRulesFiles:   m.config.GetHouseRulesFiles(),
		},
	))
}

func (m *Model) handleReviewDiffLoaded(msg reviewDiffLoadedMsg) (tea.Model, tea.Cmd) {
	prepared := m.stopPreparing()

	if msg.branchErr != nil {
		m.branchErr = msg.branchErr
		m.message = fmt.Sprintf(
			"Could not check against %s.\n\nPress Esc to try a different branch.",
			m.styles.Accent.Render(m.branch),
		)

		m.message = m.styles.Info.Padding(2).Render(m.message)
		return m, nil
	}

	if msg.err != nil {
		m.error = msg.err

		return m, nil
	}

	if msg.diff == "" {
		m.message = m.styles.Info.Padding(2).Render(
			m.nothingToReview(msg.reviewDiff) + "\n\nPress Esc to go back, or ctrl+c to exit.",
		)
		return m, nil
	}

	if msg.instruction != "" {
		m.selectedInstruction = msg.instruction
	}
	structured := m.config.GetStructuredFindings()
	system := prompt.FormatReviewSystem(m.selectedReviewer.Prompt, msg.instruction, msg.houseRules.Text)
	if structured {
		system = prompt.FormatStructuredReviewSystem(m.selectedReviewer.Prompt, msg.instruction, msg.houseRules.Text)
	}

	diff := msg.diff
	if structured {
		diff = prompt.NumberDiffLines(diff)
	}
	reviewPrompt := prompt.FormatReviewContent(msg.contextHeader, msg.stat, msg.commits, diff, msg.enclosingContext.Text)

	ctx, cancel := context.WithTimeout(context.Background(), m.llmTimeout())

	if m.reviewCancelFunc != nil {
		m.reviewCancelFunc()
	}
	m.reviewCancelFunc = cancel

	m.review = newReviewModel(*m.selectedReviewer, system, reviewPrompt, m.width, m.height, m.llm)
	m.review.setStyles(m.styles, m.isDarkMode)
	m.review.loader.TakeOver(prepared)
	m.review.showRelativeLineNumbers(m.config.GetRelativeNumber())
	m.review.setUsedModel(m.getLlmModelName())
	m.review.setStructured(structured)
	if structured {
		// As in the CLI: the model is told Bark reports these.
		m.review.setSecrets(redact.Findings(msg.diff))
	}
	m.review.setDiffNotes(msg.reviewDiff.Cut, msg.reviewDiff.Warnings)
	m.review.minSeverity = findings.Severity(m.config.GetMinSeverity())
	m.review.setLogInfo(m.paths.State, m.llmProvider)
	m.review.houseRules = houserules.LogField(m.config.GetHouseRules(), msg.houseRules)
	m.review.enclosingCut = msg.enclosingContext.DroppedSummary()
	m.review.setPricing(m.pricing)
	if structured && m.config.GetCheckFindings() {
		m.review.setCheck(m.llmTimeout(), check.Review{
			Client:     m.llm,
			ReviewDiff: msg.reviewDiff,
			Diff:       diff,
			HouseRules: msg.houseRules.Text,
			LogDir:     m.paths.State,
			Provider:   m.llmProvider,
			Model:      m.getLlmModelName(),
			Pricing:    m.pricing,
		})
	}
	m.currentView = viewReview

	return m, m.review.startReview(ctx)
}

// nothingToReview says why r, the diff of the review's source, is empty:
// nothing changed, or only files Bark ignores did.
func (m *Model) nothingToReview(r git.ReviewDiff) string {
	var hash string
	if m.selectedCommit != nil {
		hash = m.selectedCommit.Hash[:min(7, len(m.selectedCommit.Hash))]
	}

	var change, nothing string
	switch {
	case m.prNumber != "":
		change = fmt.Sprintf("pull request #%s changes", m.prNumber)
		nothing = fmt.Sprintf("Pull request #%s has no changes to review.", m.prNumber)
	case m.branch != "":
		change = fmt.Sprintf("the commits since %s change", m.branch)
		nothing = fmt.Sprintf("No changes since %s to review.", m.branch)
	case m.selectCommit:
		change = fmt.Sprintf("commit %s changes", hash)
		nothing = fmt.Sprintf("Commit %s has no changes to review.", hash)
	case m.stagedOnly:
		change = "the staged changes touch"
		nothing = "No staged changes to review."
	default:
		change = "the uncommitted changes touch"
		nothing = "No uncommitted changes to review."
	}

	if len(r.Ignored) == 0 {
		return nothing
	}
	note := r.IgnoredOnly(change)
	return strings.ToUpper(note[:1]) + note[1:] + "."
}

func (m *Model) handleCommitMessage(commitAll bool) (tea.Model, tea.Cmd) {
	return m, tea.Batch(m.startPreparing(generationPhaseLoadingMessages[:]), loadCommitDataCmd(m.config.GetCommitInstructions(), commitAll))
}

// startPreparing shows the loader, with the next screen's loading
// messages, until that screen's data is loaded.
func (m *Model) startPreparing(messages []string) tea.Cmd {
	m.preparing = true
	m.loader.SetStyles(m.styles)
	return m.loader.Start(messages)
}

// stopPreparing ends the loading screen and returns its loader for the
// screen that was loading to take over.
func (m *Model) stopPreparing() loader {
	m.preparing = false
	l := m.loader
	m.loader = loader{}
	return l
}

func (m *Model) handleCommitDataLoaded(msg commitDataLoadedMsg) (tea.Model, tea.Cmd) {
	prepared := m.stopPreparing()

	if msg.err != nil {
		m.error = msg.err
		return m, nil
	}

	if msg.diff == "" {
		m.message = "No changes to commit.\n"
		if !msg.commitAll {
			m.message += "Tip: use 'C' to commit all changes, including unstaged ones.\n"
		}

		if m.selectedTask != TaskCommit {
			m.message += "Press Esc to go back, or ctrl+c to exit."
		} else {
			m.message += "Press ctrl+c to exit."
		}

		m.message = m.styles.Info.Padding(2).Render(m.message)

		return m, nil
	}

	commitSystem := prompt.FormatCommitSystem(msg.instructions, m.hint)

	ctx, cancel := context.WithTimeout(context.Background(), m.llmTimeout())

	if m.operationCancelFunc != nil {
		m.operationCancelFunc()
	}
	m.operationCancelFunc = cancel

	commitContent := prompt.FormatCommitContent(msg.branch, msg.diff)
	m.commitChanges = newCommitChangesModel(m.llm, commitSystem, commitContent, msg.commitAll, m.width, m.height)
	m.commitChanges.setStyles(m.styles, m.isDarkMode)
	m.commitChanges.loader.TakeOver(prepared)
	m.commitChanges.showRelativeLineNumbers(m.config.GetRelativeNumber())
	m.commitChanges.displayUsedModel(m.getLlmModelName())
	m.currentView = viewCommitChanges
	return m, m.commitChanges.startCommitGeneration(ctx)
}

// handleCommitMessageRetry regenerates the commit message. It reloads the
// diff, branch and instructions first, so changes made while the commit view
// is open are part of the new prompt.
func (m *Model) handleCommitMessageRetry() (tea.Model, tea.Cmd) {
	return m.handleCommitMessage(m.commitChanges.commitAll)
}

func (m *Model) handlePRDescription() (tea.Model, tea.Cmd) {
	return m, tea.Batch(m.startPreparing(prLoadingMessages[:]), loadPRDataCmd(m.prDataParams()))
}

func (m *Model) prDataParams() prDataCmdParams {
	return prDataCmdParams{
		instructions:         m.instructionValues,
		fallbackInstructions: m.config.GetPRInstructions(),
		prNumber:             m.prNumber,
		branch:               m.branch,
		diffLimit:            git.LimitFrom(m.config),
	}
}

func (m *Model) handlePRDataLoaded(msg prDataLoadedMsg) (tea.Model, tea.Cmd) {
	prepared := m.stopPreparing()

	if msg.err != nil {
		m.error = msg.err
		return m, nil
	}

	m.pr = newPRModel(m.llm, m.width, m.height)
	m.pr.setStyles(m.styles, m.isDarkMode)
	m.pr.loader.TakeOver(prepared)
	m.pr.displayUsedModel(m.getLlmModelName())
	m.pr.setContent(
		prompt.FormatPRSystem(msg.instructions),
		msg.content,
	)
	ctx, cancel := context.WithTimeout(context.Background(), m.llmTimeout())

	if m.operationCancelFunc != nil {
		m.operationCancelFunc()
	}
	m.operationCancelFunc = cancel

	m.currentView = viewPRDescription
	return m, m.pr.startPRDescriptionGeneration(ctx)
}

func (m *Model) getLlmModelName() string {
	model, err := m.config.GetLLMModel()
	if err != nil {
		return "unknown"
	}

	return model
}

func performCommit(message string, commitAll bool) tea.Cmd {
	return func() tea.Msg {
		outChan, errChan := git.CommitChanges(context.Background(), message, commitAll)
		return commitStreamStartMsg{
			outChan: outChan,
			errChan: errChan,
		}
	}
}
