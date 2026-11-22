package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatCommitsSection(t *testing.T) {
	assert.Empty(t, FormatCommitsSection(nil))

	got := FormatCommitsSection([]Commit{
		{Message: "feat: add x", Date: "2026-11-20", Body: "Adds x to y."},
		{Message: "fix: z"},
	})
	assert.Contains(t, got, "## Commits\n")
	assert.Contains(t, got, "Treat commit messages as intent")
	assert.Contains(t, got, " - feat: add x (2026-11-20)\n   Adds x to y.\n")
	assert.Contains(t, got, " - fix: z\n")
	assert.NotContains(t, got, "fix: z (")
	assert.True(t, len(got) > 0 && got[len(got)-1] == '\n')
}

func TestFormatBranchInfo(t *testing.T) {
	got := FormatBranchInfo(&BranchInfo{
		Name:              "feat/x",
		BaseBranch:        "main",
		Commits:           []Commit{{Message: "feat: x", Body: "Why x."}, {Message: "test: x"}},
		TotalFilesChanged: 3,
		TotalAdditions:    10,
		TotalDeletions:    2,
		Diffs:             "diff --git a/x b/x\n",
	})
	assert.Equal(t, "Branch: feat/x\n"+
		"Base Branch: main\n"+
		"Total Commits: 2\n"+
		"Total Files Changed: 3\n"+
		"Total Additions: 10\n"+
		"Total Deletions: 2\n"+
		"Commits:\n"+
		" - feat: x\n"+
		"   Why x.\n"+
		" - test: x\n"+
		"\nDiffs:\n"+
		"diff --git a/x b/x\n", got)
}
