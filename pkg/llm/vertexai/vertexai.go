package vertexai

import (
	"context"

	"github.com/ionut-t/bark/pkg/llm/genai"
	g "google.golang.org/genai"
)

func New(ctx context.Context, model, project, location string) (*genai.GenAI, error) {
	return genai.New(ctx, model, g.ClientConfig{
		Backend:  g.BackendVertexAI,
		Project:  project,
		Location: location,
	})
}
