package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/utils"
	"github.com/ionut-t/bark/v2/pkg/git"
	"github.com/ionut-t/bark/v2/pkg/instructions"
	"github.com/ionut-t/bark/v2/pkg/llm"
	"github.com/ionut-t/bark/v2/pkg/llm/llm_factory"
	prompt_pkg "github.com/ionut-t/bark/v2/pkg/prompt"
	"github.com/ionut-t/bark/v2/pkg/reviewers"
	"github.com/ionut-t/coffee/styles"
)

const (
	defaultCommitLimit = 25
	ctxTimeout         = 5 * time.Minute
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
)

type commitStatusMessage struct {
	error error
}

type Model struct {
	width, height int

	error     error
	commitErr error

	currentView view

	llm     llm.LLM
	config  config.Config
	storage string

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
	instructionName     string
	skipInstruction     bool

	commitChanges commitChangesModel

	branch      string
	branchErr   error
	branchInput branchInputModel

	prNumber      string
	prNumberInput prNumberInputModel

	pr prModel

	hint string

	showHelp bool
	message  string

	reviewCancelFunc    context.CancelFunc
	operationCancelFunc context.CancelFunc

	styles     styles.Styles
	isDarkMode bool
	pendingCmd tea.Cmd

	viewport viewport.Model
}

type Options struct {
	Storage         string
	ReviewerName    string
	Instruction     string
	Branch          string
	PR              string
	SelectCommit    bool
	Config          config.Config
	StagedOnly      bool
	SkipInstruction bool
	Task            Task
	ReviewOption    ReviewOption
	Hint            string
}

func New(options Options) *Model {
	isDarkMode := styles.IsDark()

	llm, err := llm_factory.New(context.Background(), options.Config)
	if err != nil {
		return &Model{
			error: err,
		}
	}

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
		config:               options.Config,
		storage:              options.Storage,
		selectCommit:         options.SelectCommit,
		reviewerName:         options.ReviewerName,
		instructionName:      options.Instruction,
		branch:               options.Branch,
		branchInput:          newBranchInputModel(options.Branch),
		prNumber:             options.PR,
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
	m.tasks.setStyles(styles, isDarkMode)
	m.reviewOptions.setStyles(styles, isDarkMode)

	return m
}

func (m Model) Init() tea.Cmd {
	return m.pendingCmd
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.SetWidth(max(msg.Width-2, 10))
		m.viewport.SetHeight(max(msg.Height-10, 5))

		switch m.currentView {
		case viewCommits:
			m.commits.setSize(m.width, m.height)
		case viewReviewers:
			m.reviewers.setSize(m.width, m.height)
		case viewInstructions:
			m.instructions.setSize(m.width, m.height)
		case viewReview:
			m.review.setSize(m.width, m.height)
		case viewCommitChanges:
			m.commitChanges.setSize(m.width, m.height)
		case viewPRDescription:
			m.pr.setSize(m.width, m.height)
		}

	case taskSelectedMsg:
		return m.handleSelectedTask(msg.task)

	case reviewOptionSelectedMsg:
		return m.handleSelectedReviewOption(msg.option)

	case branchSelectedMsg:
		m.branch = msg.branch
		return m, utils.DispatchMsg(listReviewersMsg{})

	case listCommitsMsg:
		commits, err := git.GetCommits(defaultCommitLimit)
		if err != nil {
			m.error = err
			return m, nil
		}

		m.commits = newCommitsModel(commits)
		m.commits.setSize(m.width, m.height)
		m.commits.setStyles(m.styles, m.isDarkMode)
		m.currentView = viewCommits

	case commitSelectedMsg:
		m.selectedCommit = &msg.commit
		return m, utils.DispatchMsg(listReviewersMsg{})

	case listReviewersMsg:
		listReviewers, err := reviewers.Get(m.storage)
		if err != nil {
			m.error = err
		}
		m.reviewers = newReviewersModel(listReviewers, m.styles, m.isDarkMode)

		if m.reviewerName != "" {
			if reviewer, err := reviewers.Find(m.reviewerName, listReviewers); err == nil {
				m.selectedReviewer = reviewer
				m.reviewers = newReviewersModel(listReviewers, m.styles, m.isDarkMode)
				return m, utils.DispatchMsg(reviewerSelectedMsg{Reviewer: reviewer})
			}
		}

		m.currentView = viewReviewers
		m.reviewers = newReviewersModel(listReviewers, m.styles, m.isDarkMode)

	case reviewerSelectedMsg:
		return m.handleSelectedReviewer(msg.Reviewer)

	case instructionSelectedMsg:
		return m.handleSelectedInstruction(msg.instruction)

	case commitChangesMsg:
		// Clean up the commit context since operation completed
		if m.operationCancelFunc != nil {
			m.operationCancelFunc()
			m.operationCancelFunc = nil
		}

		m.commitErr = nil
		m.commitChanges.loading = true

		return m, performCommit(msg.message, msg.commitAll)

	case commitStatusMessage:
		m.commitChanges.loading = false

		if msg.error != nil {
			m.commitErr = msg.error
			m.viewport.SetContent(m.styles.Error.Padding(0, 2).Render(msg.error.Error()))
		} else {
			return m, tea.Quit
		}

	case prInitReadyMsg:
		return m.handlePRDescription()

	case cancelReviewOptionSelectionMsg:
		if m.individualTask {
			break
		}

		m.currentView = viewTasks
		m.selectedReviewOption = ReviewOptionNone

	case prNumberSelectedMsg:
		m.prNumber = msg.prNumber
		return m, utils.DispatchMsg(listReviewersMsg{})

	case cancelPRNumberSelectionMsg:
		m.currentView = viewReviewOptions
		m.prNumber = ""

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
		switch msg.String() {
		case "ctrl+c":
			if m.reviewCancelFunc != nil {
				m.reviewCancelFunc()
			}

			if m.operationCancelFunc != nil {
				m.operationCancelFunc()
			}

			return m, tea.Quit

		case "?":
			m.showHelp = !m.showHelp

		case "esc":
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

		case "ctrl+r":
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
					ctx, cancel := context.WithTimeout(context.Background(), ctxTimeout)

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
					ctx, cancel := context.WithTimeout(context.Background(), ctxTimeout)

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
	}

	if m.commitErr != nil {
		m.viewport, cmd = m.viewport.Update(msg)
	}

	return m, cmd
}

func (m Model) View() tea.View {
	view := tea.NewView(m.createView())
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
		if errors.Is(m.error, git.ErrNoChangesInRepository) {
			return m.styles.Info.Padding(2).Render(
				lipgloss.JoinVertical(lipgloss.Left,
					"Could not find any changes to review.",
					"This can happen when there are no commits in the repository.",
					"Stage some changes and run `bark review --stage`.",
					"\n",
					"Press `ctrl+c` to exit.",
				),
			)
		}

		if errors.Is(m.error, git.ErrNoCommitsInRepository) {
			return m.styles.Info.Padding(2).Render(
				lipgloss.JoinVertical(lipgloss.Left,
					"Could not find any commits.",
					"This can happen when there are no commits in the repository.",
					"Try running `bark review --stage` to review staged changes.",
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
		m.pr = newPRModel(m.llm, m.width, m.height)
		m.pr.setStyles(m.styles, m.isDarkMode)
		m.currentView = viewPRDescription

		return m, tea.Batch(
			m.pr.Init(),
			utils.DispatchMsg(prInitReadyMsg{}),
		)
	}

	return m, nil
}

func (m *Model) handleSelectedReviewer(reviewer *reviewers.Reviewer) (tea.Model, tea.Cmd) {
	m.selectedReviewer = reviewer
	listInstructions, err := instructions.Get(m.storage)

	if m.skipInstruction {
		return m, utils.DispatchMsg(instructionSelectedMsg{instruction: ""})
	}

	if m.instructionName != "" {
		if instruction, err := instructions.Find(m.instructionName, listInstructions); err == nil {
			m.selectedInstruction = instruction.Prompt
			return m, utils.DispatchMsg(instructionSelectedMsg{instruction: instruction.Prompt})
		}
	}

	if err != nil {
		m.error = err
	}

	if len(listInstructions) == 0 {
		return m, utils.DispatchMsg(instructionSelectedMsg{instruction: ""})
	}

	m.instructions = newInstructionsModel(listInstructions, m.styles, m.isDarkMode)
	m.currentView = viewInstructions

	return m, nil
}

func (m *Model) handleSelectedReviewOption(option ReviewOption) (tea.Model, tea.Cmd) {
	m.selectedReviewOption = option

	switch m.selectedReviewOption {
	case ReviewOptionCurrentChanges, ReviewOptionStagedChanges:
		return m, utils.DispatchMsg(listReviewersMsg{})
	case ReviewOptionCommit:
		m.selectCommit = true
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
	var diff string
	var err error

	if m.prNumber != "" {
		diff, err = git.GetPRDiff(m.prNumber)
	} else if m.branch != "" {
		diff, m.branchErr = git.GetBranchDiff(m.branch, m.config.GetMaxDiffLines())

		if m.branchErr != nil {
			m.message = fmt.Sprintf(
				"Could not check against %s.\n\nPress Esc to try a different branch.",
				m.styles.Accent.Render(m.branch),
			)

			m.message = m.styles.Info.Padding(2).Render(m.message)
			return m, nil
		}

	} else if m.selectCommit {
		diff, err = git.GetDiff(m.selectedCommit.Hash)
	} else {
		diff, err = git.GetWorkingTreeDiff(!m.stagedOnly)
	}

	if err != nil {
		m.error = err

		return m, nil
	}

	prompt := m.selectedReviewer.Prompt

	if instruction != "" {
		m.selectedInstruction = instruction
		prompt = fmt.Sprintf("%s\nFollow the instructions below when analysing code:\n\n%s", prompt, instruction)
	}

	prompt = fmt.Sprintf("%s%s---\n\n**Code to review:**\n%s", prompt, prompt_pkg.FormattingRequirements, diff)

	ctx, cancel := context.WithTimeout(context.Background(), ctxTimeout)

	if m.reviewCancelFunc != nil {
		m.reviewCancelFunc()
	}
	m.reviewCancelFunc = cancel

	m.review = newReviewModel(*m.selectedReviewer, prompt, m.width, m.height, m.llm)
	m.review.setStyles(m.styles, m.isDarkMode)
	m.currentView = viewReview

	return m, m.review.startReview(ctx)
}

func (m *Model) handleCommitMessage(commitAll bool) (tea.Model, tea.Cmd) {
	instructions := m.config.GetCommitInstructions()

	diff, err := git.GetWorkingTreeDiff(commitAll)
	if err != nil {
		m.error = err
		return m, nil
	}

	if diff == "" {
		m.message = "No changes to commit.\n"
		if !commitAll {
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

	prompt := instructions
	if m.hint != "" {
		prompt += "\nBased on the following hint, determine the type of changes (e.g., feature, fix, refactor, docs) for the commit message.\n"
		prompt += "Commit message hint: " + m.hint
	}

	prompt += "\n\n" + diff

	ctx, cancel := context.WithTimeout(context.Background(), ctxTimeout)

	if m.operationCancelFunc != nil {
		m.operationCancelFunc()
	}
	m.operationCancelFunc = cancel

	m.commitChanges = newCommitChangesModel(m.llm, prompt, commitAll, m.width, m.height)
	m.commitChanges.setStyles(m.styles, m.isDarkMode)
	m.currentView = viewCommitChanges
	return m, m.commitChanges.startCommitGeneration(ctx)
}

func (m *Model) handleCommitMessageRetry() (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithTimeout(context.Background(), ctxTimeout)

	if m.operationCancelFunc != nil {
		m.operationCancelFunc()
	}

	m.operationCancelFunc = cancel

	return m, m.commitChanges.startCommitGeneration(ctx)
}

func (m *Model) handlePRDescription() (tea.Model, tea.Cmd) {
	instructions := m.config.GetPRInstructions()

	branchInfo, err := git.GetBranchInfo(m.branch, m.config.GetMaxDiffLines())
	if err != nil {
		m.error = err
		return m, nil
	}

	prompt := fmt.Sprintf(
		"%s**Analyze the following changes and generate an appropriate PR description:**\n\n%s",
		instructions,
		git.FormatBranchInfo(branchInfo),
	)

	m.pr.setPrompt(prompt)
	ctx, cancel := context.WithTimeout(context.Background(), ctxTimeout)

	if m.operationCancelFunc != nil {
		m.operationCancelFunc()
	}
	m.operationCancelFunc = cancel

	return m, m.pr.startPRDescriptionGeneration(ctx)
}

func performCommit(message string, commitAll bool) tea.Cmd {
	return func() tea.Msg {
		err := git.CommitChanges(message, commitAll)
		return commitStatusMessage{error: err}
	}
}
