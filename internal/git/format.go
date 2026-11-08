package git

import (
	"fmt"
	"regexp"
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
	sb.WriteString("_The diff below is the cumulative net result of these commits. Treat commit messages as intent, not final state — a commit may add code that a later commit refactors or removes, so do not flag changes the final diff does not contain._\n\n")
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

// FormatPRHeader returns a markdown header for a pull request.
func FormatPRHeader(meta *PRMeta) string {
	header := fmt.Sprintf("## PR #%d: %s\n\n", meta.Number, meta.Title)
	if meta.Body != "" {
		header += fmt.Sprintf("### Description\n\n%s\n\n", meta.Body)
	}
	return header
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

var (
	goFileInDiff = regexp.MustCompile(`(?m)^diff --git a/.+ b/.+\.go$`)
	goDirective  = regexp.MustCompile(`(?m)^go\s+(\S+)\s*(?://.*)?$`)
)

// FormatGoVersionHeader returns a markdown header naming the Go version in
// goMod, if diff changes Go files. Models otherwise judge a change against
// the Go they were trained on and report newer standard library APIs (such
// as errors.AsType, added in Go 1.26) as nonexistent, or mock the version
// itself as made up.
func FormatGoVersionHeader(goMod []byte, diff string) string {
	m := goDirective.FindSubmatch(goMod)
	if m == nil || !goFileInDiff.MatchString(diff) {
		return ""
	}
	return fmt.Sprintf("## Go version\n\n"+
		"go.mod targets Go %s, a released version that may be newer than your training data. "+
		"Its language features and standard library APIs, and those of earlier versions, exist: "+
		"don't report one as missing or nonexistent because you don't know it. "+
		"Don't comment on the Go version itself.\n\n", m[1])
}
