package debuglog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogWritesPromptsRawOutputAndStats(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	now := time.Date(2026, 11, 8, 14, 30, 5, 0, time.UTC)
	l, err := start(dir, "review", Meta{
		Fields: []Field{{"provider", "gemini"}, {"model", "gemini-3.6-flash"}, {"reviewer", "Spock"}},
		System: "SYSTEM", Prompt: "PROMPT",
	}, now)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "logs", "2026-11-08T14-30-05.000-review.log"), l.Path())

	l.Write(`{"type":"intro",`, nil)
	l.Write(`"text":"Hi"}`, &llm.Usage{InputTokens: 100, OutputTokens: 20, TotalTokens: 150})
	l.Finish(errors.New("boom"))

	content, err := os.ReadFile(l.Path())
	require.NoError(t, err)
	got := string(content)

	assert.True(t, strings.HasPrefix(got,
		"kind:     review\n"+
			"time:     2026-11-08T14:30:05Z\n"+
			"provider: gemini\n"+
			"model:    gemini-3.6-flash\n"+
			"reviewer: Spock\n"), got)
	assert.Contains(t, got, "===== system =====\nSYSTEM\n")
	assert.Contains(t, got, "===== prompt =====\nPROMPT\n")
	assert.Contains(t, got, "===== response (raw) =====\n"+`{"type":"intro","text":"Hi"}`+"\n")
	assert.Contains(t, got, "tokens:       100 input, 20 output, 150 total\n              30 other (thinking)\n")
	assert.Contains(t, got, "error:        boom\n")
	assert.Regexp(t, `first output: \d+\.\ds\n`, got)
}

func TestLogWithoutOutput(t *testing.T) {
	t.Parallel()

	l, err := start(t.TempDir(), "commit", Meta{}, time.Now())
	require.NoError(t, err)
	l.Finish(nil)

	content, err := os.ReadFile(l.Path())
	require.NoError(t, err)
	assert.Contains(t, string(content), "first output: none\n")
	assert.NotContains(t, string(content), "error:")
}

// Callers use the log without checking whether logging is on.
func TestNilLogDoesNothing(t *testing.T) {
	t.Parallel()

	var l *Log
	l.Write("x", &llm.Usage{})
	l.Finish(nil)
	assert.Empty(t, l.Path())
}

func TestStartIsOffByDefault(t *testing.T) {
	t.Setenv(EnvVar, "")
	dir := t.TempDir()

	l, err := Start(dir, "review", Meta{})
	require.NoError(t, err)
	assert.Nil(t, l)
	assert.NoDirExists(t, filepath.Join(dir, "logs"))

	t.Setenv(EnvVar, "1")
	l, err = Start(dir, "review", Meta{})
	require.NoError(t, err)
	require.NotNil(t, l)
	l.Finish(nil)
	assert.FileExists(t, l.Path())
}
