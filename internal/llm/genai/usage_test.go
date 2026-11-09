package genai

import (
	"testing"

	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/stretchr/testify/assert"
	"google.golang.org/genai"
)

// Gemini's prompt count includes cached tokens; thinking is in the total
// but not in the output.
func TestUsageOf(t *testing.T) {
	t.Parallel()

	assert.Nil(t, usageOf(nil))

	got := usageOf(&genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:        10_000,
		CachedContentTokenCount: 6_000,
		CandidatesTokenCount:    300,
		ThoughtsTokenCount:      2_000,
		TotalTokenCount:         12_300,
	})
	assert.Equal(t, &llm.Usage{InputTokens: 10_000, OutputTokens: 300, TotalTokens: 12_300, CachedInputTokens: 6_000}, got)
	assert.Equal(t, int64(2_000), got.ThinkingTokens())
}
