package gemini

import (
	"context"

	"github.com/ionut-t/bark/v2/internal/llm/genai"
	g "google.golang.org/genai"
)

func New(ctx context.Context, model, apiKey, effort string) (*genai.GenAI, error) {
	return genai.New(ctx, model, effort, g.ClientConfig{
		Backend: g.BackendGeminiAPI,
		APIKey:  apiKey,
	})
}
