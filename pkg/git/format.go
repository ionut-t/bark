package git

import (
	"fmt"
	"strings"
)

// FormatBranchInfo formats a BranchInfo into a human-readable string.
func FormatBranchInfo(branch *BranchInfo) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Branch: %s\n", branch.Name)
	fmt.Fprintf(&sb, "Base Branch: %s\n", branch.BaseBranch)
	fmt.Fprintf(&sb, "Total Commits: %d\n", len(branch.Commits))
	fmt.Fprintf(&sb, "Total Files Changed: %d\n", branch.TotalFilesChanged)
	fmt.Fprintf(&sb, "Total Additions: %d\n", branch.TotalAdditions)
	fmt.Fprintf(&sb, "Total Deletions: %d\n", branch.TotalDeletions)
	sb.WriteString("Commits:\n")

	for _, commit := range branch.Commits {
		fmt.Fprintf(&sb, " - %s\n", commit.Message)
		if commit.Body != "" {
			fmt.Fprintf(&sb, "   %s\n", commit.Body)
		}
	}

	sb.WriteString("\nDiffs:\n")
	sb.WriteString(branch.Diffs)

	return sb.String()
}
