package plain

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/debuglog"
	"github.com/ionut-t/bark/v2/internal/enclosing"
	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/ionut-t/bark/v2/internal/instructions"
	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/ionut-t/bark/v2/internal/llm/llm_factory"
	"github.com/ionut-t/bark/v2/internal/prompt"
	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/ionut-t/bark/v2/internal/utils"
)

const gitTimeout = 30 * time.Second

// ReviewOptions configures the plain text review runner.
type ReviewOptions struct {
	Diff              *string
	ReviewerName      string
	Instruction       string
	SkipInstruction   bool
	Storage           string
	Config            config.Config
	Stream            bool
	WithPRDescription bool

	// Diff source flags (used when Diff is empty)
	Staged bool
	All    bool
	Branch string
	Hash   string
	PR     string
}

// CommitOptions configures the plain text commit runner.
type CommitOptions struct {
	Diff   *string
	All    bool
	Hint   string
	Config config.Config
}

// PROptions configures the plain text PR runner.
type PROptions struct {
	Diff         *string
	Branch       string
	PR           string
	Instructions string
	Config       config.Config
}

// RunReview runs a code review and writes the output to stdout.
func RunReview(opts ReviewOptions) error {
	var reviewDiff git.ReviewDiff

	if opts.Diff == nil {
		gitCtx, gitCancel := context.WithTimeout(context.Background(), gitTimeout)
		defer gitCancel()

		maxLines := opts.Config.GetMaxDiffLines()
		var diffParams git.ReviewDiffParams
		switch {
		case opts.PR != "":
			diffParams = git.PRDiff(opts.PR).WithMaxLines(maxLines)
			if opts.WithPRDescription {
				diffParams = diffParams.WithPRDescription()
			}
		case opts.Branch != "":
			diffParams = git.BranchDiff(opts.Branch).WithMaxLines(maxLines)
		case opts.Hash != "":
			diffParams = git.CommitDiff(opts.Hash).WithMaxLines(maxLines)
		default:
			diffParams = git.WorkingTreeDiff(opts.Staged).WithMaxLines(maxLines)
		}

		var err error
		reviewDiff, err = git.GetReviewDiff(gitCtx, diffParams)
		if err != nil {
			return err
		}
	} else {
		reviewDiff.Diff = *opts.Diff
	}

	if reviewDiff.Diff == "" {
		return fmt.Errorf("no diff content available")
	}

	reviewer, err := resolveReviewer(opts.ReviewerName, opts.Storage)
	if err != nil {
		return err
	}

	var reviewInstructions string
	if !opts.SkipInstruction {
		var err error
		reviewInstructions, err = resolveInstructions(opts.Instruction, opts.Storage)
		if err != nil {
			return err
		}
	}
	structured := opts.Config.GetStructuredFindings()
	system := prompt.FormatReviewSystem(reviewer.Prompt, reviewInstructions)
	if structured {
		system = prompt.FormatStructuredReviewSystem(reviewer.Prompt, reviewInstructions)
	}

	var enclosingContext string
	if opts.Config.GetContextEnrichment() && !reviewDiff.SkipEnrichment {
		enclosingCtx, enclosingCancel := context.WithTimeout(context.Background(), gitTimeout)
		var err error
		enclosingContext, err = enclosing.DeclarationsForDiff(enclosingCtx, reviewDiff.Diff, reviewDiff.Ref)
		enclosingCancel()
		if err != nil {
			// Gracefully continue without enclosing context if extraction fails
			enclosingContext = ""
		}
	}

	diff := reviewDiff.Diff
	if structured {
		diff = prompt.NumberDiffLines(diff)
	}
	promptText := prompt.FormatReviewContent(reviewDiff.ContextHeader, reviewDiff.Stat, reviewDiff.Commits, diff, enclosingContext)

	client, provider, err := llm_factory.New(context.Background(), opts.Config)
	if err != nil {
		return fmt.Errorf("error creating LLM client: %w", err)
	}

	llmCtx, llmCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer llmCancel()

	model, _ := opts.Config.GetLLMModel()
	debugLog, err := debuglog.Start(opts.Storage, "review", debuglog.Meta{
		Fields: []debuglog.Field{
			{Key: "provider", Value: provider},
			{Key: "model", Value: model},
			{Key: "reviewer", Value: reviewer.Name},
			{Key: "structured", Value: strconv.FormatBool(structured)},
		},
		System: system,
		Prompt: promptText,
	})
	// Stderr, so the review on stdout stays clean for pipes.
	if err != nil {
		fmt.Fprintf(os.Stderr, "bark: debug log: %v\n", err)
	} else if debugLog != nil {
		fmt.Fprintf(os.Stderr, "bark: debug log: %s\n", debugLog.Path())
	}

	out := io.Writer(os.Stdout)
	if opts.Stream {
		return streamResponse(llmCtx, client, system, promptText, out, structured, debugLog)
	}

	return fullResponse(llmCtx, client, system, promptText, out, structured, debugLog)
}

// RunCommit generates a commit message and writes it to stdout.
func RunCommit(opts CommitOptions) error {
	var diff string

	if opts.Diff == nil {
		gitCtx, gitCancel := context.WithTimeout(context.Background(), gitTimeout)
		defer gitCancel()

		var err error
		diff, err = git.GetWorkingTreeDiff(gitCtx, opts.All)
		if err != nil {
			return err
		}
	} else {
		diff = *opts.Diff
	}

	if diff == "" {
		return fmt.Errorf("no changes to generate a commit message for")
	}

	commitInstructions, err := utils.GetInstructions(".bark/commit.md", opts.Config.GetCommitInstructions())
	if err != nil {
		return err
	}
	commitSystem := prompt.FormatCommitSystem(commitInstructions, opts.Hint)

	client, _, err := llm_factory.New(context.Background(), opts.Config)
	if err != nil {
		return fmt.Errorf("error creating LLM client: %w", err)
	}

	llmCtx, llmCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer llmCancel()

	result, err := client.Generate(llmCtx, commitSystem, diff)
	if err != nil {
		return fmt.Errorf("error generating commit message: %w", err)
	}

	fmt.Print(utils.RemoveCodeFences(result.Content))
	fmt.Println()

	return nil
}

// RunPR generates a PR description and writes it to stdout.
func RunPR(opts PROptions) error {
	prInstructions, err := resolvePRInstructions(opts.Instructions, opts.Config)
	if err != nil {
		return err
	}

	var content string

	switch {
	case opts.Diff != nil:
		content = *opts.Diff
	default:
		gitCtx, gitCancel := context.WithTimeout(context.Background(), gitTimeout)
		defer gitCancel()

		var err error
		if opts.PR != "" {
			content, err = git.GetPRInfo(gitCtx, opts.PR)
		} else {
			var branchInfo *git.BranchInfo
			branchInfo, err = git.GetBranchInfo(gitCtx, opts.Branch, opts.Config.GetMaxDiffLines())
			if err == nil {
				content = git.FormatBranchInfo(branchInfo)
			}
		}
		if err != nil {
			return err
		}
	}

	prSystem := prompt.FormatPRSystem(prInstructions)

	client, _, err := llm_factory.New(context.Background(), opts.Config)
	if err != nil {
		return fmt.Errorf("error creating LLM client: %w", err)
	}

	llmCtx, llmCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer llmCancel()

	result, err := client.Generate(llmCtx, prSystem, content)
	if err != nil {
		return fmt.Errorf("error generating PR description: %w", err)
	}

	fmt.Print(result.Content)
	fmt.Println()

	return nil
}

// streamResponse streams the review to out. A structured review is rendered
// to Markdown as it arrives.
func streamResponse(ctx context.Context, client llm.LLM, system, promptText string, out io.Writer, structured bool, debugLog *debuglog.Log) error {
	responseChan, errChan := client.Stream(ctx, system, promptText)

	var renderer findings.Renderer
	for chunk := range responseChan {
		debugLog.Write(chunk.Content, chunk.Usage)
		if structured {
			_, _ = fmt.Fprint(out, renderer.Write(chunk.Content))
		} else {
			_, _ = fmt.Fprint(out, chunk.Content)
		}
	}
	if structured {
		_, _ = fmt.Fprint(out, renderer.Close())
	}
	_, _ = fmt.Fprintln(out)

	err := <-errChan
	debugLog.Finish(err)
	if err != nil {
		return fmt.Errorf("error during review: %w", err)
	}

	return nil
}

func fullResponse(ctx context.Context, client llm.LLM, system, promptText string, out io.Writer, structured bool, debugLog *debuglog.Log) error {
	response, err := client.Generate(ctx, system, promptText)
	debugLog.Write(response.Content, response.Usage)
	debugLog.Finish(err)
	if err != nil {
		return fmt.Errorf("error during review: %w", err)
	}

	content := response.Content
	if structured {
		content = findings.Markdown(findings.Parse(content))
	}

	_, _ = fmt.Fprint(out, content)
	_, _ = fmt.Fprintln(out)

	return nil
}

// resolveReviewer loads a reviewer from a file path, by name, or from .bark/reviewer.md.
func resolveReviewer(name, storage string) (*reviewers.Reviewer, error) {
	if name != "" {
		if _, err := os.Stat(name); err == nil {
			return reviewers.FromFile(name)
		}
		reviewerList, err := reviewers.Get(storage)
		if err != nil {
			return nil, fmt.Errorf("error loading reviewers: %w", err)
		}
		r, err := reviewers.Find(name, reviewerList)
		if err != nil {
			return nil, fmt.Errorf("reviewer not found: %w", err)
		}
		return r, nil
	}

	if _, err := os.Stat(".bark/reviewer.md"); err == nil {
		return reviewers.FromFile(".bark/reviewer.md")
	}

	return nil, fmt.Errorf("reviewer is required in plain mode (use --as or add .bark/reviewer.md)")
}

// resolveInstructions returns the instruction text from a file path, by name, raw text, or .bark/review.md.
func resolveInstructions(instruction, storage string) (string, error) {
	if instruction != "" {
		if _, err := os.Stat(instruction); err == nil {
			content, err := os.ReadFile(instruction)
			if err != nil {
				return "", fmt.Errorf("error reading instructions file: %w", err)
			}
			return string(content), nil
		}

		instructionList, _ := instructions.Get(storage)
		if instr, err := instructions.Find(instruction, instructionList); err == nil {
			return instr.Prompt, nil
		}

		return instruction, nil
	}

	return utils.ReadLocalOverride(".bark/review.md")
}

// resolvePRInstructions returns the instruction text from a file path.
func resolvePRInstructions(instruction string, cfg config.Config) (string, error) {
	if instruction != "" {
		if _, err := os.Stat(instruction); err == nil {
			content, err := os.ReadFile(instruction)
			if err != nil {
				return "", fmt.Errorf("error reading PR instructions file: %w", err)
			}
			return string(content), nil
		}
		return instruction, nil
	}

	return utils.GetInstructions(".bark/pr.md", cfg.GetPRInstructions())
}
