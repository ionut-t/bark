package tui

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ionut-t/bark/internal/config"
	"github.com/ionut-t/bark/internal/utils"
	"github.com/ionut-t/bark/pkg/git"
	"github.com/ionut-t/bark/pkg/instructions"
	"github.com/ionut-t/bark/pkg/llm"
	"github.com/ionut-t/bark/pkg/llm/llm_factory"
	"github.com/ionut-t/bark/pkg/reviewers"
	"github.com/ionut-t/coffee/styles"
)

const defaultCommitLimit = 25
const ctxTimeout = 3 * time.Minute

//go:embed format.md
var formatingRequirements string

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
)

type Model struct {
	width, height int

	error       error
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

	pr prModel

	hint string

	showHelp bool
	message  string

	reviewCancelFunc    context.CancelFunc
	operationCancelFunc context.CancelFunc
}

type Options struct {
	Storage         string
	ReviewerName    string
	Instruction     string
	Branch          string
	SelectCommit    bool
	Config          config.Config
	StagedOnly      bool
	SkipInstruction bool
	Task            Task
	ReviewOption    ReviewOption
	Hint            string
}

func New(options Options) *Model {
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

	return &Model{
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
		stagedOnly:           options.StagedOnly,
		skipInstruction:      options.SkipInstruction,
		tasks:                newTasksModel(),
		selectedTask:         options.Task,
		reviewOptions:        newReviewOptionsModel(),
		selectedReviewOption: options.ReviewOption,
		individualTask:       options.Task != TaskNone,
		hint:                 options.Hint,
	}
}

func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd

	title := "Bark AI"
	switch m.selectedTask {
	case TaskReview:
		cmds = append(cmds, utils.DispatchMsg(taskSelectedMsg{task: TaskReview}))
		title += " - Code Review"
	case TaskCommit:
		cmds = append(cmds, utils.DispatchMsg(taskSelectedMsg{task: TaskCommit}))
		title += " - Commit Changes"
	case TaskPRDescription:
		cmds = append(cmds, utils.DispatchMsg(taskSelectedMsg{task: TaskPRDescription}))
		title += " - PR Description"
	}

	cmds = append(cmds, tea.SetWindowTitle(title))

	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

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
		m.currentView = viewCommits

	case commitSelectedMsg:
		m.selectedCommit = &msg.commit
		return m, utils.DispatchMsg(listReviewersMsg{})

	case listReviewersMsg:
		listReviewers, err := reviewers.Get(m.storage)
		if err != nil {
			m.error = err
		}
		m.reviewers = newReviewersModel(listReviewers)

		if m.reviewerName != "" {
			if reviewer, err := reviewers.Find(m.reviewerName, listReviewers); err == nil {
				m.selectedReviewer = reviewer
				m.reviewers = newReviewersModel(listReviewers)
				return m, utils.DispatchMsg(reviewerSelectedMsg{Reviewer: reviewer})
			}
		}

		m.currentView = viewReviewers
		m.reviewers = newReviewersModel(listReviewers)

	case reviewerSelectedMsg:
		return m.handleSelectedReviewer(msg.Reviewer)

	case instructionSelectedMsg:
		return m.handleSelectedInstruction(msg.Instruction)

	case commitChangesMsg:
		// Clean up the commit context since operation completed
		if m.operationCancelFunc != nil {
			m.operationCancelFunc()
			m.operationCancelFunc = nil
		}

		if err := git.CommitChanges(msg.message, msg.commitAll); err != nil {
			m.error = err
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

	case cancelBranchSelectionMsg:
		m.currentView = viewReviewOptions
		m.branch = ""

	case cancelReviewerSelectionMsg:
		m.selectedReviewer = nil

		if m.selectCommit {
			m.currentView = viewCommits
		} else if m.branch != "" {
			m.currentView = viewBranchInput
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
			m.message = ""
			if m.branchErr != nil {
				m.currentView = viewBranchInput
				m.branchErr = nil
				return m, nil
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
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

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
				(m.currentView == viewReview && !m.selectCommit) {
				return m.handleCommitMessage(msg.String() == "C")
			}
		}
	}

	switch m.currentView {
	case viewTasks:
		commands, cmd := m.tasks.Update(msg)
		m.tasks = commands.(tasksModel)
		cmds = append(cmds, cmd)

	case viewReviewOptions:
		reviewOptions, cmd := m.reviewOptions.Update(msg)
		m.reviewOptions = reviewOptions.(reviewOptionsModel)
		cmds = append(cmds, cmd)

	case viewCommits:
		var cmd tea.Cmd
		m.commits, cmd = m.commits.Update(msg)
		cmds = append(cmds, cmd)

	case viewReviewers:
		var cmd tea.Cmd
		m.reviewers, cmd = m.reviewers.Update(msg)
		cmds = append(cmds, cmd)

	case viewInstructions:
		i, cmd := m.instructions.Update(msg)
		m.instructions = i.(instructionsModel)
		cmds = append(cmds, cmd)

	case viewReview:
		review, cmd := m.review.Update(msg)
		m.review = review.(reviewModel)
		cmds = append(cmds, cmd)

	case viewCommitChanges:
		commitChanges, cmd := m.commitChanges.Update(msg)
		m.commitChanges = commitChanges
		cmds = append(cmds, cmd)

	case viewPRDescription:
		pr, cmd := m.pr.Update(msg)
		m.pr = pr
		cmds = append(cmds, cmd)

	case viewBranchInput:
		input, cmd := m.branchInput.Update(msg)
		m.branchInput = input.(branchInputModel)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.error != nil {
		if errors.Is(m.error, git.ErrNoChangesInRepository) {
			return styles.Info.Padding(2).Render(
				lipgloss.JoinVertical(lipgloss.Left,
					"Could not find any changes to review.",
					"This can happen when there are no commits in the repository.",
					"Stage some changes and run `bark --stage`.",
					"\n",
					"Press `ctrl+c` to exit.",
				),
			)
		}

		if errors.Is(m.error, git.ErrNoCommitsInRepository) {
			return styles.Info.Padding(2).Render(
				lipgloss.JoinVertical(lipgloss.Left,
					"Could not find any commits.",
					"This can happen when there are no commits in the repository.",
					"Try running `bark --stage` to review staged changes.",
					"\n",
					"Press `ctrl+c` to exit.",
				),
			)
		}

		return styles.Error.Padding(2).Render("Error: " + m.error.Error() + "\n" + "Press `ctrl+c` to exit.")
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
			return reviewHelp(m.width, m.selectCommit)
		}

		return m.review.View()

	case viewCommitChanges:
		return m.commitChanges.View()

	case viewPRDescription:
		return m.pr.View()

	case viewBranchInput:
		return m.branchInput.View()
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
		return m, utils.DispatchMsg(instructionSelectedMsg{Instruction: ""})
	}

	if m.instructionName != "" {
		if instruction, err := instructions.Find(m.instructionName, listInstructions); err == nil {
			m.selectedInstruction = instruction.Prompt
			return m, utils.DispatchMsg(instructionSelectedMsg{Instruction: instruction.Prompt})
		}
	}

	if err != nil {
		m.error = err
	}

	if len(listInstructions) == 0 {
		return m, utils.DispatchMsg(instructionSelectedMsg{Instruction: ""})
	}

	m.instructions = newInstructionsModel(listInstructions, m.storage)
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
	}

	return m, nil
}

func (m *Model) handleSelectedInstruction(instruction string) (tea.Model, tea.Cmd) {
	var diff string
	var err error

	if m.branch != "" {
		diff, m.branchErr = git.GetBranchDiff(m.branch)

		if m.branchErr != nil {
			m.message = fmt.Sprintf(
				"Could not check against %s.\n\nPress Esc to try a different branch.",
				styles.Accent.Render(m.branch),
			)

			m.message = styles.Info.Padding(2).Render(m.message)
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

	var prompt string
	prompt = fmt.Sprintf("%s\n\n%s", formatingRequirements, m.selectedReviewer.Prompt)

	if instruction != "" {
		m.selectedInstruction = instruction
		prompt = fmt.Sprintf("%s\n\nFollow the instructions below when analysing code:\n\n%s", prompt, instruction)
	}

	prompt = fmt.Sprintf("%s\n\n---\n\n**Code to review:**\n\n%s", prompt, diff)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	if m.reviewCancelFunc != nil {
		m.reviewCancelFunc()
	}
	m.reviewCancelFunc = cancel

	m.review = newReviewModel(*m.selectedReviewer, prompt, m.width, m.height, m.llm)
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

		m.message += "Press Esc to go back."

		m.message = styles.Info.Padding(2).Render(m.message)

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

	branchInfo, err := git.GetBranchInfo(m.branch)

	if err != nil {
		m.error = err
		return m, nil
	}

	prompt := fmt.Sprintf(
		"%s**Analyze the following changes and generate an appropriate PR description:**\n\n%s",
		instructions,
		formatBranchInfo(branchInfo),
	)

	m.pr.setPrompt(prompt)
	ctx, cancel := context.WithTimeout(context.Background(), ctxTimeout)

	if m.operationCancelFunc != nil {
		m.operationCancelFunc()
	}
	m.operationCancelFunc = cancel

	return m, m.pr.startPRDescriptionGeneration(ctx)
}
