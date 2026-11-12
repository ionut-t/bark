package prompt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatStructuredReviewSystem(t *testing.T) {
	t.Parallel()

	system := FormatStructuredReviewSystem("You are Spock.", "Focus on errors.")

	assert.True(t, strings.HasPrefix(system, "# Persona\n\nYou are Spock.\n\n# Output Format"))
	assert.Contains(t, system, `{"type":"intro","text":"..."}`)
	assert.Contains(t, system, `{"type":"verdict","text":"..."}`)
	assert.True(t, strings.HasSuffix(system, "Follow the instructions below when analysing code:\n\nFocus on errors."))
	assert.NotContains(t, system, "unified diff format", "the free-text formatting rules must not leak in")
	assert.NotContains(t, FormatStructuredReviewSystem("p", ""), "Follow the instructions")
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
