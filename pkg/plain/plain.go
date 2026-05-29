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
	Model           string
	Storage         string
	Config          config.Config
	Stream          bool

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
	Model        string
	Instructions string
	Config       config.Config
}

// RunReview runs a code review and writes the output to stdout.
func RunReview(opts ReviewOptions) error {
	var diff string

	if opts.Diff == nil {
		var err error
		switch {
		case opts.PR != "":
			diff, err = git.GetPRDiff(opts.PR)
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

	opts.Config.OverrideModel(opts.Model)

	reviewer, err := resolveReviewer(opts.ReviewerName, opts.Storage)
	if err != nil {
		return err
	}

	promptText := reviewer.Prompt

	if !opts.SkipInstruction {
		instructions, err := resolveInstructions(opts.Instruction, opts.Storage)
		if err != nil {
			return err
		}
		if instructions != "" {
			promptText = fmt.Sprintf("%s\nFollow the instructions below when analysing code:\n\n%s", promptText, instructions)
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
	opts.Config.OverrideModel(opts.Model)

	prInstructions, err := resolvePRInstructions(opts.Instructions, opts.Config)
	if err != nil {
		return err
	}

	var content string

	switch {
	case opts.Diff != nil:
		content = *opts.Diff
	case opts.PR != "":
		var err error
		content, err = git.GetPRInfo(opts.PR)
		if err != nil {
			return err
		}
	default:
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

// resolveInstructions returns the instruction text from a file path, by name, raw text, or .bark/instructions.md.
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

	if content, err := os.ReadFile(".bark/instructions.md"); err == nil {
		return string(content), nil
	}

	return "", nil
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

	if content, err := os.ReadFile(".bark/pull_request_description.md"); err == nil && len(content) > 0 {
		return string(content), nil
	}

	return cfg.GetPRInstructions(), nil
}

// Errf writes a formatted error message to stderr.
func Errf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
}
