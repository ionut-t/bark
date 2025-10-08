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
	"github.com/ionut-t/bark/pkg/git"
	"github.com/ionut-t/bark/pkg/instructions"
	"github.com/ionut-t/bark/pkg/llm"
	"github.com/ionut-t/bark/pkg/llm/llm_factory"
	"github.com/ionut-t/bark/pkg/reviewers"
	"github.com/ionut-t/coffee/styles"
)

const defaultCommitLimit = 25

//go:embed format.md
var formatingRequirements string

type view int

const (
	viewInit view = iota
	viewCommits
	viewReviewers
	viewInstructions
	viewReview
	viewCommitChanges
)

type Model struct {
	width, height       int
	error               error
	commits             commitsModel
	selectedCommit      *git.Commit
	reviewers           reviewersModel
	selectedReviewer    *reviewers.Reviewer
	review              reviewModel
	currentView         view
	llm                 llm.LLM
	config              config.Config
	storage             string
	selectCommit        bool
	reviewerName        string
	instructions        instructionsModel
	selectedInstruction string
	instructionName     string
	branch              string
	commitChanges       commitChangesModel
	diff                string
	showHelp            bool
	stagedOnly          bool
	message             string

	reviewCancelFunc context.CancelFunc
	commitCancelFunc context.CancelFunc
}

type Options struct {
	Storage      string
	ReviewerName string
	Instruction  string
	Branch       string
	SelectCommit bool
	Config       config.Config
	StagedOnly   bool
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

	return &Model{
		width:           80,
		height:          24,
		currentView:     currentView,
		llm:             llm,
		config:          options.Config,
		storage:         options.Storage,
		selectCommit:    options.SelectCommit,
		reviewerName:    options.ReviewerName,
		instructionName: options.Instruction,
		branch:          options.Branch,
		stagedOnly:      options.StagedOnly,
	}
}

func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd

	if m.error != nil {
		return nil
	}

	if m.selectCommit {
		cmd := func() tea.Msg {
			return listCommitsMsg{}
		}

		cmds = append(cmds, cmd)
	} else {
		cmd := func() tea.Msg {
			return listReviewersMsg{}
		}

		cmds = append(cmds, cmd)
	}

	cmds = append(cmds, tea.SetWindowTitle("Bark - AI Code Reviewer"))

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
		}

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
		return m, func() tea.Msg {
			return listReviewersMsg{}
		}

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
				return m, func() tea.Msg {
					return reviewerSelectedMsg{Reviewer: reviewer}
				}
			}
		}

		m.currentView = viewReviewers
		m.reviewers = newReviewersModel(listReviewers)

	case reviewerSelectedMsg:
		m.selectedReviewer = msg.Reviewer
		listInstructions, err := instructions.Get(m.storage)

		if m.instructionName != "" {
			if instruction, err := instructions.Find(m.instructionName, listInstructions); err == nil {
				m.selectedInstruction = instruction.Prompt
				return m, func() tea.Msg {
					return instructionSelectedMsg{Instruction: instruction.Prompt}
				}
			}
		}

		if err != nil {
			m.error = err
		}

		if len(listInstructions) == 0 {
			return m, func() tea.Msg {
				return instructionSelectedMsg{Instruction: ""}
			}
		}

		m.instructions = newInstructionsModel(listInstructions, m.storage)
		m.currentView = viewInstructions

		return m, nil

	case instructionSelectedMsg:
		var err error

		if m.branch != "" {
			m.diff, err = git.GetBranchDiff(m.branch)
		} else if m.selectCommit {
			m.diff, err = git.GetDiff(m.selectedCommit.Hash)
		} else {
			m.diff, err = git.GetWorkingTreeDiff(!m.stagedOnly)
		}

		if err != nil {
			m.error = err

			return m, nil
		}

		var prompt string
		prompt = fmt.Sprintf("%s\n\n%s", formatingRequirements, m.selectedReviewer.Prompt)

		if msg.Instruction != "" {
			m.selectedInstruction = msg.Instruction
			prompt = fmt.Sprintf("%s\n\nFollow the instructions below when analysing code:\n\n%s", prompt, msg.Instruction)
		}

		prompt = fmt.Sprintf("%s\n\n---\n\n**Code to review:**\n\n%s", prompt, m.diff)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

		if m.reviewCancelFunc != nil {
			m.reviewCancelFunc()
		}
		m.reviewCancelFunc = cancel

		m.review = newReviewModel(*m.selectedReviewer, prompt, m.width, m.height, m.llm)
		m.currentView = viewReview

		return m, m.review.startReview(ctx)

	case commitChangesMsg:
		// Clean up the commit context since operation completed
		if m.commitCancelFunc != nil {
			m.commitCancelFunc()
			m.commitCancelFunc = nil
		}

		if err := git.CommitChanges(msg.message, msg.commitAll); err != nil {
			m.error = err
		} else {
			return m, tea.Quit
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			if m.reviewCancelFunc != nil {
				m.reviewCancelFunc()
			}

			if m.commitCancelFunc != nil {
				m.commitCancelFunc()
			}

			return m, tea.Quit

		case "?":
			m.showHelp = !m.showHelp

		case "esc":
			m.message = ""

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
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)

					if m.commitCancelFunc != nil {
						m.commitCancelFunc()
					}

					m.commitCancelFunc = cancel

					return m, m.commitChanges.startCommitGeneration(ctx)
				}
			}

		case "c", "C":
			m.message = ""
			m.error = nil

			if m.currentView == viewReview && !m.selectCommit {
				instructions := m.config.GetCommitInstructions()

				commitAll := msg.String() == "C"

				diff, err := git.GetWorkingTreeDiff(commitAll)

				if err != nil {
					m.error = err
					return m, nil
				}

				if diff == "" {
					m.message = "No changes to commit.\n"
					if !commitAll {
						m.message += "Tip: use 'C' to commit all changes, including unstaged and untracked ones.\n"
					}

					m.message += "Press Esc to go back."

					m.message = styles.Info.Padding(2).Render(m.message)

					return m, nil
				}

				prompt := instructions + "\n\n" + diff

				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)

				// Cancel previous commit operation if any
				if m.commitCancelFunc != nil {
					m.commitCancelFunc()
				}
				m.commitCancelFunc = cancel

				m.commitChanges = newCommitChangesModel(m.llm, prompt, commitAll)
				m.currentView = viewCommitChanges
				return m, m.commitChanges.startCommitGeneration(ctx)
			}
		}
	}

	switch m.currentView {
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
	case viewCommits:
		return m.commits.View()
	case viewReviewers:
		return m.reviewers.View()
	case viewInstructions:
		return m.instructions.View()
	case viewReview:
		if m.showHelp {
			return reviewHelp(m.width, m.selectCommit)
		}

		return m.review.View()
	case viewCommitChanges:
		return m.commitChanges.View()
	default:
		return ""
	}
}
