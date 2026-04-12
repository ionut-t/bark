package plain

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/utils"
	"github.com/ionut-t/bark/v2/pkg/git"
	"github.com/ionut-t/bark/v2/pkg/instructions"
	"github.com/ionut-t/bark/v2/pkg/llm"
	"github.com/ionut-t/bark/v2/pkg/llm/llm_factory"
	"github.com/ionut-t/bark/v2/pkg/prompt"
	"github.com/ionut-t/bark/v2/pkg/reviewers"
)

// ReviewOptions configures the plain text review runner.
type ReviewOptions struct {
	Diff            *string
	ReviewerName    string
	Instruction     string
	SkipInstruction bool
	Storage         string
	Config          config.Config
	Stream          bool

	// Diff source flags (used when Diff is empty)
	Staged bool
	All    bool
	Branch string
	Hash   string
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
	Diff   *string
	Branch string
	Config config.Config
}

// RunReview runs a code review and writes the output to stdout.
func RunReview(opts ReviewOptions) error {
	var diff string

	if opts.Diff == nil {
		var err error
		switch {
		case opts.Hash != "":
			diff, err = git.GetDiff(opts.Hash)
		case opts.Branch != "":
			diff, err = git.GetBranchDiff(opts.Branch, opts.Config.GetMaxDiffLines())
		case opts.Staged:
			diff, err = git.GetWorkingTreeDiff(false)
		default:
			diff, err = git.GetWorkingTreeDiff(true)
		}
		if err != nil {
			return err
		}
	} else {
		diff = *opts.Diff
	}

	if diff == "" {
		return fmt.Errorf("no diff content available")
	}

	if opts.ReviewerName == "" {
		return fmt.Errorf("reviewer is required in plain mode (use --as)")
	}

	reviewerList, err := reviewers.Get(opts.Storage)
	if err != nil {
		return fmt.Errorf("error loading reviewers: %w", err)
	}

	reviewer, err := reviewers.Find(opts.ReviewerName, reviewerList)
	if err != nil {
		return fmt.Errorf("reviewer not found: %w", err)
	}

	promptText := reviewer.Prompt

	if !opts.SkipInstruction && opts.Instruction != "" {
		instructionList, _ := instructions.Get(opts.Storage)
		if instr, err := instructions.Find(opts.Instruction, instructionList); err == nil {
			promptText = fmt.Sprintf("%s\nFollow the instructions below when analysing code:\n\n%s", promptText, instr.Prompt)
		} else {
			// Use the instruction value as raw instruction text
			promptText = fmt.Sprintf("%s\nFollow the instructions below when analysing code:\n\n%s", promptText, opts.Instruction)
		}
	}

	promptText = fmt.Sprintf("%s%s---\n\n**Code to review:**\n%s", promptText, prompt.FormattingRequirements, diff)

	client, err := llm_factory.New(context.Background(), opts.Config)
	if err != nil {
		return fmt.Errorf("error creating LLM client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if opts.Stream {
		return streamResponse(ctx, client, promptText)
	}

	return fullResponse(ctx, client, promptText)
}

// RunCommit generates a commit message and writes it to stdout.
func RunCommit(opts CommitOptions) error {
	var diff string

	if opts.Diff == nil {
		var err error
		diff, err = git.GetWorkingTreeDiff(opts.All)
		if err != nil {
			return err
		}
	} else {
		diff = *opts.Diff
	}

	if diff == "" {
		return fmt.Errorf("no changes to generate a commit message for")
	}

	promptText := opts.Config.GetCommitInstructions()
	if opts.Hint != "" {
		promptText += "\nBased on the following hint, determine the type of changes (e.g., feature, fix, refactor, docs) for the commit message.\n"
		promptText += "Commit message hint: " + opts.Hint
	}
	promptText += "\n\n" + diff

	client, err := llm_factory.New(context.Background(), opts.Config)
	if err != nil {
		return fmt.Errorf("error creating LLM client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	result, err := client.Generate(ctx, promptText)
	if err != nil {
		return fmt.Errorf("error generating commit message: %w", err)
	}

	fmt.Print(utils.RemoveCodeFences(result))
	fmt.Println()

	return nil
}

// RunPR generates a PR description and writes it to stdout.
func RunPR(opts PROptions) error {
	prInstructions := opts.Config.GetPRInstructions()

	var content string

	if opts.Diff != nil {
		content = *opts.Diff
	} else {
		branchInfo, err := git.GetBranchInfo(opts.Branch, opts.Config.GetMaxDiffLines())
		if err != nil {
			return err
		}
		content = git.FormatBranchInfo(branchInfo)
	}

	promptText := fmt.Sprintf(
		"%s**Analyze the following changes and generate an appropriate PR description:**\n\n%s",
		prInstructions,
		content,
	)

	client, err := llm_factory.New(context.Background(), opts.Config)
	if err != nil {
		return fmt.Errorf("error creating LLM client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	result, err := client.Generate(ctx, promptText)
	if err != nil {
		return fmt.Errorf("error generating PR description: %w", err)
	}

	fmt.Print(result)
	fmt.Println()

	return nil
}

// streamResponse streams LLM response chunks to stdout.
func streamResponse(ctx context.Context, client llm.LLM, promptText string) error {
	responseChan, errChan := client.Stream(ctx, promptText)

	for chunk := range responseChan {
		fmt.Print(chunk.Content)
	}
	fmt.Println()

	if err := <-errChan; err != nil {
		return fmt.Errorf("error during review: %w", err)
	}

	return nil
}

func fullResponse(ctx context.Context, client llm.LLM, promptText string) error {
	response, err := client.Generate(ctx, promptText)
	if err != nil {
		return fmt.Errorf("error during review: %w", err)
	}

	fmt.Print(response)
	fmt.Println()

	return nil
}

// Errf writes a formatted error message to stderr.
func Errf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
}
