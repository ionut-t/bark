package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/utils"
	"github.com/ionut-t/bark/v2/pkg/git"
	"github.com/ionut-t/bark/v2/pkg/instructions"
	"github.com/ionut-t/bark/v2/pkg/reviewers"
)

// The commands in this file offload all blocking I/O onto goroutines via tea.Cmd, so Update() never
// blocks the Bubble Tea event loop. Each command performs its work and returns
// a *LoadedMsg that the Update handler turns into a pure state transition.

type commitsLoadedMsg struct {
	commits []git.Commit
	err     error
}

func loadCommitsCmd(limit int) tea.Cmd {
	return func() tea.Msg {
		commits, err := git.GetCommits(limit)
		return commitsLoadedMsg{commits: commits, err: err}
	}
}

type reviewersLoadedMsg struct {
	reviewers    []reviewers.Reviewer
	reviewersErr error
	fileReviewer *reviewers.Reviewer
	fileErr      error
}

func loadReviewersCmd(storage string) tea.Cmd {
	return func() tea.Msg {
		list, listErr := reviewers.Get(storage)
		fileReviewer, fileErr := reviewers.FromFile(".bark/reviewer.md")
		return reviewersLoadedMsg{
			reviewers:    list,
			reviewersErr: listErr,
			fileReviewer: fileReviewer,
			fileErr:      fileErr,
		}
	}
}

type reviewInstructionsLoadedMsg struct {
	instructions    []instructions.Instruction
	instructionsErr error
	override        string
	overrideErr     error
}

func loadReviewInstructionsCmd(storage string) tea.Cmd {
	return func() tea.Msg {
		list, listErr := instructions.Get(storage)
		override, overrideErr := utils.ReadLocalOverride(".bark/review.md")
		return reviewInstructionsLoadedMsg{
			instructions:    list,
			instructionsErr: listErr,
			override:        override,
			overrideErr:     overrideErr,
		}
	}
}

type reviewDiffLoadedMsg struct {
	instruction string
	diff        string
	err         error
	branchErr   error
}

type reviewDiffCmdParams struct {
	prNumber     string
	branch       string
	maxLines     uint32
	selectCommit bool
	commitHash   string
	stagedOnly   bool
	instruction  string
}

func loadReviewDiffCmd(params reviewDiffCmdParams) tea.Cmd {
	return func() tea.Msg {
		var diff string
		var err, branchErr error

		switch {
		case params.prNumber != "":
			diff, err = git.GetPRDiff(params.prNumber)
		case params.branch != "":
			diff, branchErr = git.GetBranchDiff(params.branch, params.maxLines)
		case params.selectCommit:
			diff, err = git.GetDiff(params.commitHash)
		default:
			diff, err = git.GetWorkingTreeDiff(!params.stagedOnly)
		}

		return reviewDiffLoadedMsg{
			instruction: params.instruction,
			diff:        diff,
			err:         err,
			branchErr:   branchErr,
		}
	}
}

type commitDataLoadedMsg struct {
	instructions string
	diff         string
	commitAll    bool
	err          error
}

func loadCommitDataCmd(fallbackInstructions string, commitAll bool) tea.Cmd {
	return func() tea.Msg {
		instr, err := utils.GetInstructions(".bark/commit.md", fallbackInstructions)
		if err != nil {
			return commitDataLoadedMsg{commitAll: commitAll, err: err}
		}

		diff, err := git.GetWorkingTreeDiff(commitAll)
		return commitDataLoadedMsg{
			instructions: instr,
			diff:         diff,
			commitAll:    commitAll,
			err:          err,
		}
	}
}

type prDataLoadedMsg struct {
	instructions string
	content      string
	err          error
}

type prDataCmdParams struct {
	fallbackInstructions string
	prNumber             string
	branch               string
	maxLines             uint32
}

func loadPRDataCmd(params prDataCmdParams) tea.Cmd {
	return func() tea.Msg {
		instr, err := utils.GetInstructions(".bark/pr.md", params.fallbackInstructions)
		if err != nil {
			return prDataLoadedMsg{err: err}
		}

		var content string
		if params.prNumber != "" {
			content, err = git.GetPRInfo(params.prNumber)
			if err != nil {
				return prDataLoadedMsg{err: err}
			}
		} else {
			branchInfo, infoErr := git.GetBranchInfo(params.branch, params.maxLines)
			if infoErr != nil {
				return prDataLoadedMsg{err: infoErr}
			}
			content = git.FormatBranchInfo(branchInfo)
		}

		return prDataLoadedMsg{instructions: instr, content: content}
	}
}
