package git

import (
	"fmt"
	"slices"
	"strings"
)

// FormatCommitsSection formats a list of commits into a "## Commits" markdown section.
// Returns an empty string when the list is empty.
func FormatCommitsSection(commits []Commit) string {
	if len(commits) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Commits\n")
	for _, c := range commits {
		if c.Date != "" {
			fmt.Fprintf(&sb, " - %s (%s)\n", c.Message, c.Date)
		} else {
			fmt.Fprintf(&sb, " - %s\n", c.Message)
		}
		if c.Body != "" {
			fmt.Fprintf(&sb, "   %s\n", c.Body)
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

var noisyBranches = [...]string{"main", "master", "develop", "development", "head"}

// FormatBranchHeader returns a "## Branch:" header for meaningful branch names.
// Returns empty string for trunk/default branches that carry no intent signal.
func FormatBranchHeader(branch string) string {
	if branch == "" || slices.Contains(noisyBranches[:], strings.ToLower(branch)) {
		return ""
	}
	return fmt.Sprintf("## Branch: %s\n\n", branch)
}

// FormatPRHeader returns a short markdown header line for a pull request.
func FormatPRHeader(meta *PRMeta) string {
	return fmt.Sprintf("## PR #%d: %s\n\n", meta.Number, meta.Title)
}

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
