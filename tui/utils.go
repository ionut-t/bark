package tui

import (
	"fmt"
	"strings"

	"github.com/ionut-t/bark/pkg/git"
)

func formatBranchInfo(branch *git.BranchInfo) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Branch: %s\n", branch.Name))
	sb.WriteString(fmt.Sprintf("Base Branch: %s\n", branch.BaseBranch))
	sb.WriteString(fmt.Sprintf("Total Commits: %d\n", len(branch.Commits)))
	sb.WriteString(fmt.Sprintf("Total Additions: %d\n", branch.TotalAdditions))
	sb.WriteString(fmt.Sprintf("Total Deletions: %d\n", branch.TotalDeletions))
	sb.WriteString("Commits:\n")

	for _, commit := range branch.Commits {
		sb.WriteString(fmt.Sprintf(" - %s\n", commit.Message))
		if commit.Body != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", commit.Body))
		}
	}

	sb.WriteString("\nDiffs:\n")
	sb.WriteString(branch.Diffs)

	return sb.String()
}
