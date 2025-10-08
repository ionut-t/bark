package llm

import (
	"context"
	"time"
)

type Response struct {
	Content string
	Time    time.Time
}

type LLM interface {
	Stream(ctx context.Context, prompt string) (<-chan Response, <-chan error)
	Generate(ctx context.Context, prompt string) (string, error)
}
