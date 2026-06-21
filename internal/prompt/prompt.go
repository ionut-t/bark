package prompt

import (
	_ "embed"
	"fmt"

	"github.com/ionut-t/bark/v2/internal/git"
)

//go:embed format.md
var formattingRequirements string

// FormatReviewSystem builds the system prompt for a code review.
func FormatReviewSystem(reviewerPrompt, instructions string) string {
	system := reviewerPrompt + "\n" + formattingRequirements
	if instructions != "" {
		system += fmt.Sprintf("\nFollow the instructions below when analysing code:\n\n%s", instructions)
	}
	return system
}

// FormatReviewContent assembles the user-facing review prompt from fetched git context.
func FormatReviewContent(contextHeader, stat string, commits []git.Commit, diff string) string {
	commitsSection := git.FormatCommitsSection(commits)
	statSection := ""
	if stat != "" {
		statSection = fmt.Sprintf("## Files Changed\n%s\n\n", stat)
	}
	return fmt.Sprintf("%s%s%s**Code to review:**\n%s", contextHeader, commitsSection, statSection, diff)
}

// FormatCommitSystem builds the system prompt for commit message generation.
func FormatCommitSystem(instructions, hint string) string {
	if hint == "" {
		return instructions
	}
	return instructions +
		"\nBased on the following hint, determine the type of changes (e.g., feature, fix, refactor, docs) for the commit message.\n" +
		"Commit message hint: " + hint
}

// FormatPRSystem builds the system prompt for PR description generation.
func FormatPRSystem(instructions string) string {
	return instructions + "**Analyze the following changes and generate an appropriate PR description:**"
}
