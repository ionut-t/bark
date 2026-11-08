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
