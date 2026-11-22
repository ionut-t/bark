package prreview

import (
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var keyDiff = "diff --git a/a.go b/a.go\n" +
	"--- a/a.go\n" +
	"+++ b/a.go\n" +
	"@@ -1,1 +1,2 @@\n" +
	" package a\n" +
	"+var key = \"" + "AKIA" + "IOSFODNN7EXAMPLE" + "\"\n"

// The model saw the key masked (see redact.Wrap). A suggestion that moves
// the key out of the code still replaces the real line: its original is
// matched against the masked line.
func TestBuildSuggestionForMaskedOriginal(t *testing.T) {
	t.Parallel()

	got := Build([]findings.Event{fix(2, 0, `var key = "[REDACTED:aws-access-key-id]"`, `var key = os.Getenv("AWS_ACCESS_KEY_ID")`)}, keyDiff)
	require.Len(t, got.Comments, 1)
	assert.Equal(t, suggestionBody(`var key = os.Getenv("AWS_ACCESS_KEY_ID")`), got.Comments[0].Body)
}

// A suggestion that holds a placeholder would write it into the code if
// applied, so it's shown as plain code, not offered as one to apply.
func TestBuildNoSuggestionBlockWithPlaceholder(t *testing.T) {
	t.Parallel()

	code := `const key = "[REDACTED:aws-access-key-id]"`
	got := Build([]findings.Event{fix(2, 0, `var key = "[REDACTED:aws-access-key-id]"`, code)}, keyDiff)
	require.Len(t, got.Comments, 1)
	assert.Equal(t, plainCodeBody(code), got.Comments[0].Body)
}
