package check

import (
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/stretchr/testify/assert"
)

func TestPromptShowsHouseRules(t *testing.T) {
	t.Parallel()

	in := input()
	in.HouseRules = "<file path=\"AGENTS.md\">\nNever log tokens.\n</file>\n"
	fs := []findings.Finding{{File: "a.go", Line: 4, Severity: findings.High, Title: "t", Body: "b"}}

	got := Prompt(in, fs)

	assert.Contains(t, got, "\n## House rules\n")
	assert.Contains(t, got, in.HouseRules)
	assert.Less(t, strings.Index(got, "## House rules"), strings.Index(got, "## Findings"))

	assert.NotContains(t, Prompt(input(), fs), "House rules", "no section without rules")
}

func TestSystem_HouseRuleBreaksArentPreferences(t *testing.T) {
	t.Parallel()

	assert.Contains(t, System(), "the repository's house rules")
	assert.Contains(t, System(), "breaks a rule the house rules state")
}
