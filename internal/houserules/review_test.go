package houserules

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestForReviewReadsRulesNearChangedFiles(t *testing.T) {
	r := repo{
		"AGENTS.md":           "root rules",
		"pkg/api/AGENTS.md":   "api rules",
		"pkg/other/AGENTS.md": "unrelated rules",
	}
	diff := "diff --git a/pkg/api/handler.go b/pkg/api/handler.go\n" +
		"--- a/pkg/api/handler.go\n" +
		"+++ b/pkg/api/handler.go\n" +
		"@@ -1 +1 @@\n-a\n+b\n"

	rules := ForReview(context.Background(), r.read, diff, []string{"AGENTS.md"})

	assert.Equal(t, []string{"pkg/api/AGENTS.md", "AGENTS.md"}, paths(rules))
	assert.Contains(t, rules.Text, "api rules")
	assert.Contains(t, rules.Text, "root rules")
	assert.NotContains(t, rules.Text, "unrelated rules")
}

func TestForReviewWithoutRules(t *testing.T) {
	rules := ForReview(context.Background(), repo{}.read, "", nil)
	assert.Empty(t, rules.Text)
	assert.Empty(t, rules.Sources)
}
