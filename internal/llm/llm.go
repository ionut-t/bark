package llm

import (
	"context"
	"time"
)

type Usage struct {
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
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
