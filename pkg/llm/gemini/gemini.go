package gemini

import (
	"context"

	"github.com/ionut-t/bark/pkg/llm/genai"
	g "google.golang.org/genai"
)

func New(ctx context.Context, model string, apiKey string) (*genai.GenAI, error) {
	return genai.New(ctx, model, g.ClientConfig{
		Backend: g.BackendGeminiAPI,
		APIKey:  apiKey,
	})
}
