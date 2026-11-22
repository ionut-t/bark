package prompt

import (
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/stretchr/testify/assert"
)

func TestFormatStructuredReviewSystem(t *testing.T) {
	t.Parallel()

	system := FormatStructuredReviewSystem("You are Spock.", "Focus on errors.", "")

	assert.True(t, strings.HasPrefix(system, "# Persona\n\nYou are Spock.\n\n# Output Format"))
	assert.Contains(t, system, `{"type":"intro","text":"..."}`)
	assert.Contains(t, system, `{"type":"verdict","text":"..."}`)
	assert.True(t, strings.HasSuffix(system, "Follow the instructions below when analysing code:\n\nFocus on errors."))
	assert.NotContains(t, system, "unified diff format", "the free-text formatting rules must not leak in")
	assert.NotContains(t, FormatStructuredReviewSystem("p", "", ""), "Follow the instructions")
}

// Bark reports the secrets it masks itself, so the model is told not to
// report a placeholder. A secret Bark doesn't recognise reaches the model
// as it is, and nothing else reports it, so the model must.
func TestStructuredReviewSystemSecrets(t *testing.T) {
	t.Parallel()

	system := FormatStructuredReviewSystem("p", "", "")

	assert.Contains(t, system, "[REDACTED:aws-access-key-id]")
	assert.NotContains(t, system, "don't report a hard-coded secret as a finding", "only placeholders are Bark's to report")
	assert.Contains(t, system, "Report any other hard-coded secret")
}

func TestFormatCommitContent(t *testing.T) {
	t.Parallel()

	diff := "diff --git a/x.go b/x.go\n+foo\n"

	tests := []struct {
		name   string
		branch string
		want   string
	}{
		{"feature branch is prepended", "fix/login-timeout", "## Branch: fix/login-timeout\n\n" + diff},
		{"trunk branch is omitted", "main", diff},
		{"detached HEAD is omitted", "HEAD", diff},
		{"unknown branch is omitted", "", diff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, FormatCommitContent(tt.branch, diff))
		})
	}
}

func TestReviewSystemHouseRules(t *testing.T) {
	t.Parallel()

	rules := "<file path=\"AGENTS.md\">\nNever log tokens.\n</file>\n"
	for name, format := range map[string]func(string, string, string) string{
		"free text":  FormatReviewSystem,
		"structured": FormatStructuredReviewSystem,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			system := format("You are Spock.", "Focus on errors.", rules)
			assert.Contains(t, system, "# House rules")
			assert.True(t, strings.HasSuffix(system, rules), "the rules come last")
			assert.Less(t, strings.Index(system, "Focus on errors."), strings.Index(system, "# House rules"))
			assert.Contains(t, system, "follow the instructions")

			assert.NotContains(t, format("You are Spock.", "Focus on errors.", ""), "House rules")
		})
	}
}

func TestFormatReviewSystem(t *testing.T) {
	got := FormatReviewSystem("You are Yoda.", "", "")
	assert.True(t, strings.HasPrefix(got, "You are Yoda.\n"+formattingRequirements))
	assert.NotContains(t, got, "Follow the instructions below")
	assert.NotContains(t, got, "# House rules")

	got = FormatReviewSystem("You are Yoda.", "Check errors.", "<file path=\"AGENTS.md\">x</file>")
	instr := strings.Index(got, "Follow the instructions below when analysing code:\n\nCheck errors.")
	rules := strings.Index(got, "# House rules")
	assert.Positive(t, instr)
	assert.Greater(t, rules, instr, "house rules come after the instructions")
	assert.True(t, strings.HasSuffix(got, "<file path=\"AGENTS.md\">x</file>"))
}

func TestFormatReviewContent(t *testing.T) {
	got := FormatReviewContent("## Branch: feat/x\n\n", "a.go | 2 +-", []git.Commit{{Message: "feat: x"}}, "DIFF", "CONTEXT\n")

	assert.True(t, strings.HasPrefix(got, "## Branch: feat/x\n\n## Commits\n"))
	assert.Contains(t, got, "## Files Changed\na.go | 2 +-\n\n")
	assert.True(t, strings.HasSuffix(got, "CONTEXT\n**Code to review:**\nDIFF"))

	assert.Equal(t, "**Code to review:**\nDIFF", FormatReviewContent("", "", nil, "DIFF", ""))
}

func TestFormatCommitSystem(t *testing.T) {
	assert.Equal(t, "Write commits.", FormatCommitSystem("Write commits.", ""))

	got := FormatCommitSystem("Write commits.", "fixes login")
	assert.True(t, strings.HasPrefix(got, "Write commits.\n"))
	assert.True(t, strings.HasSuffix(got, "Commit message hint: fixes login"))
}

func TestFormatPRSystem(t *testing.T) {
	assert.Equal(t,
		"PR rules.\n**Analyze the following changes and generate an appropriate PR description:**",
		FormatPRSystem("PR rules.\n"))
}
