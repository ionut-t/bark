package llm

import (
	"context"
	"time"
)

// Usage is one call's token counts, made the same across providers.
type Usage struct {
	// InputTokens counts every prompt token, including CachedInputTokens
	// and CacheWriteTokens.
	InputTokens int64
	// OutputTokens counts the answer. OpenAI and Anthropic include thinking
	// here; Gemini doesn't (see ThinkingTokens).
	OutputTokens int64
	TotalTokens  int64
	// CachedInputTokens are input tokens read from the provider's prompt
	// cache, billed at a lower price (Gemini, Anthropic, OpenAI).
	CachedInputTokens int64
	// CacheWriteTokens are input tokens written to the prompt cache
	// (Anthropic, OpenAI).
	CacheWriteTokens int64
}

// ThinkingTokens counts tokens in the total that are neither input nor
// output: Gemini's thinking, which it leaves out of OutputTokens.
func (u Usage) ThinkingTokens() int64 {
	return max(0, u.TotalTokens-u.InputTokens-u.OutputTokens)
}

type Response struct {
	Content string
	Time    time.Time
	Usage   *Usage
}

type LLM interface {
	Stream(ctx context.Context, system, prompt string) (<-chan Response, <-chan error)
	Generate(ctx context.Context, system, prompt string) (Response, error)
}
