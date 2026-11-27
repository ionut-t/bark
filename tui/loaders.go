package tui

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/enclosing"
	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/ionut-t/bark/v2/internal/houserules"
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
		ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
		defer cancel()
		fileReviewer, fileErr := reviewers.FromRepo(ctx)
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
	// reviewDiff is everything else GetReviewDiff returned; Bark's check
	// reads the files it points to.
	reviewDiff       git.ReviewDiff
	instruction      string
	diff             string
	stat             string
	commits          []git.Commit
	contextHeader    string
	enclosingContext enclosing.Context
	houseRules       houserules.Rules
	err              error
	branchErr        error
}

type reviewDiffCmdParams struct {
	prNumber          string
	branch            string
	diffLimit         git.DiffLimit
	selectCommit      bool
	commitHash        string
	stagedOnly        bool
	instruction       string
	withPRDescription bool
	contextEnrichment bool
	// houseRules reads the repository's house rules from houseRulesFiles
	// (empty means the defaults).
	houseRules      bool
	houseRulesFiles []string
}

func loadReviewDiffCmd(params reviewDiffCmdParams) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
		defer cancel()

		var diffParams git.ReviewDiffParams
		switch {
		case params.prNumber != "":
			diffParams = git.PRDiff(params.prNumber).WithLimit(params.diffLimit)
			if params.withPRDescription {
				diffParams = diffParams.WithPRDescription()
			}
		case params.branch != "":
			diffParams = git.BranchDiff(params.branch).WithLimit(params.diffLimit)
		case params.selectCommit:
			diffParams = git.CommitDiff(params.commitHash).WithLimit(params.diffLimit)
		default:
			diffParams = git.WorkingTreeDiff(params.stagedOnly).WithLimit(params.diffLimit)
		}

		result, err := git.GetReviewDiff(ctx, diffParams)

		if branchErr, ok := errors.AsType[*git.BranchDiffError](err); ok {
			return reviewDiffLoadedMsg{instruction: params.instruction, branchErr: branchErr}
		}

		var rules houserules.Rules
		if params.houseRules && err == nil {
			rules = houserules.ForReview(ctx, result.RepoFile, result.Diff, params.houseRulesFiles)
			result.Warnings = append(result.Warnings, rules.Warnings()...)
		}

		var enclosingContext enclosing.Context
		if params.contextEnrichment && err == nil {
			var ctxErr error
			enclosingContext, ctxErr = enclosing.ForReview(ctx, result)
			if ctxErr != nil {
				enclosingContext = enclosing.Context{}
			}
		}

		return reviewDiffLoadedMsg{
			reviewDiff:       result,
			instruction:      params.instruction,
			diff:             result.Diff,
			stat:             result.Stat,
			commits:          result.Commits,
			contextHeader:    result.ContextHeader,
			enclosingContext: enclosingContext,
			houseRules:       rules,
			err:              err,
		}
	}
}

type commitDataLoadedMsg struct {
	instructions string
	diff         string
	branch       string
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
		branch, _ := git.GetCurrentBranch(ctx)
		return commitDataLoadedMsg{
			instructions: instr,
			diff:         diff,
			branch:       branch,
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
	// instructions are the --instructions values; when given they replace
	// .bark/pr.md and fallbackInstructions.
	instructions         []string
	fallbackInstructions string
	prNumber             string
	branch               string
	diffLimit            git.DiffLimit
}

func loadPRDataCmd(params prDataCmdParams) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
		defer cancel()

		instr, err := resolvePRInstructions(params.instructions, params.fallbackInstructions)
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
			branchInfo, infoErr := git.GetBranchInfo(ctx, params.branch, params.diffLimit)
			if infoErr != nil {
				return prDataLoadedMsg{err: infoErr}
			}
			content = git.FormatBranchInfo(branchInfo)
		}

		return prDataLoadedMsg{instructions: instr, content: content}
	}
}

// resolvePRInstructions returns the PR instructions given by the
// --instructions values (file paths or raw text), or the repo's .bark/pr.md
// or fallback when there are none.
func resolvePRInstructions(values []string, fallback string) (string, error) {
	if len(values) > 0 {
		return instructions.Resolve(values, nil)
	}
	return utils.GetInstructions(".bark/pr.md", fallback)
}
