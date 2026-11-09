package openai

import (
	"testing"

	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
)

// OpenAI's input count already includes cached tokens, and its output
// count includes reasoning.
func TestUsageOf(t *testing.T) {
	t.Parallel()

	got := usageOf(responses.ResponseUsage{
		InputTokens:        4_000,
		InputTokensDetails: responses.ResponseUsageInputTokensDetails{CachedTokens: 3_000, CacheWriteTokens: 500},
		OutputTokens:       700,
		TotalTokens:        4_700,
	})
	assert.Equal(t, &llm.Usage{InputTokens: 4_000, OutputTokens: 700, TotalTokens: 4_700, CachedInputTokens: 3_000, CacheWriteTokens: 500}, got)
	assert.Zero(t, got.ThinkingTokens())
}
