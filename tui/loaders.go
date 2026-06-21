package tui

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/ionut-t/bark/v2/internal/instructions"
	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/ionut-t/bark/v2/internal/utils"
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
		ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
		defer cancel()
		commits, err := git.GetCommits(ctx, limit)
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
	instruction   string
	diff          string
	stat          string
	commits       []git.Commit
	contextHeader string
	err           error
	branchErr     error
}

type reviewDiffCmdParams struct {
	prNumber          string
	branch            string
	maxLines          uint32
	selectCommit      bool
	commitHash        string
	stagedOnly        bool
	instruction       string
	withPRDescription bool
}

func loadReviewDiffCmd(params reviewDiffCmdParams) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
		defer cancel()

		var diffParams git.ReviewDiffParams
		switch {
		case params.prNumber != "":
			diffParams = git.PRDiff(params.prNumber).WithMaxLines(params.maxLines)
			if params.withPRDescription {
				diffParams = diffParams.WithPRDescription()
			}
		case params.branch != "":
			diffParams = git.BranchDiff(params.branch).WithMaxLines(params.maxLines)
		case params.selectCommit:
			diffParams = git.CommitDiff(params.commitHash).WithMaxLines(params.maxLines)
		default:
			diffParams = git.WorkingTreeDiff(params.stagedOnly).WithMaxLines(params.maxLines)
		}

		result, err := git.GetReviewDiff(ctx, diffParams)

		if branchErr, ok := errors.AsType[*git.BranchDiffError](err); ok {
			return reviewDiffLoadedMsg{instruction: params.instruction, branchErr: branchErr}
		}

		return reviewDiffLoadedMsg{
			instruction:   params.instruction,
			diff:          result.Diff,
			stat:          result.Stat,
			commits:       result.Commits,
			contextHeader: result.ContextHeader,
			err:           err,
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
		ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
		defer cancel()

		instr, err := utils.GetInstructions(".bark/commit.md", fallbackInstructions)
		if err != nil {
			return commitDataLoadedMsg{commitAll: commitAll, err: err}
		}

		diff, err := git.GetWorkingTreeDiff(ctx, commitAll)
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
		ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
		defer cancel()

		instr, err := utils.GetInstructions(".bark/pr.md", params.fallbackInstructions)
		if err != nil {
			return prDataLoadedMsg{err: err}
		}

		var content string
		if params.prNumber != "" {
			content, err = git.GetPRInfo(ctx, params.prNumber)
			if err != nil {
				return prDataLoadedMsg{err: err}
			}
		} else {
			branchInfo, infoErr := git.GetBranchInfo(ctx, params.branch, params.maxLines)
			if infoErr != nil {
				return prDataLoadedMsg{err: infoErr}
			}
			content = git.FormatBranchInfo(branchInfo)
		}

		return prDataLoadedMsg{instructions: instr, content: content}
	}
}
