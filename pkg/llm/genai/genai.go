package genai

import (
	"context"
	"fmt"
	"time"

	"github.com/ionut-t/bark/pkg/llm"
	"google.golang.org/genai"
)

type GenAI struct {
	model  string
	client *genai.Client
}

func New(ctx context.Context, model string, config genai.ClientConfig) (*GenAI, error) {
	client, err := genai.NewClient(ctx, &config)

	if err != nil {
		return nil, err
	}

	return &GenAI{
		client: client,
		model:  model,
	}, nil
}

func (g *GenAI) Stream(ctx context.Context, prompt string) (<-chan llm.Response, <-chan error) {
	out := make(chan llm.Response)
	errChan := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errChan)

		// Check if context is already cancelled before starting
		if ctx.Err() != nil {
			errChan <- ctx.Err()
			return
		}

		contents := genai.Text(prompt)
		stream := g.client.Models.GenerateContentStream(ctx, g.model, contents, &genai.GenerateContentConfig{})

		for resp, err := range stream {
			select {
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			default:
			}

			if err != nil {
				errChan <- err
				return
			}

			if len(resp.Candidates) > 0 {
				if content := resp.Candidates[0].Content; content != nil {
					for _, part := range content.Parts {
						// Send response, but also watch for cancellation
						select {
						case out <- llm.Response{
							Content: part.Text,
							Time:    time.Now(),
						}:
						case <-ctx.Done():
							errChan <- ctx.Err()
							return
						}
					}
				}
			}
		}
	}()

	return out, errChan
}

func (g *GenAI) Generate(ctx context.Context, prompt string) (string, error) {
	timeout := 30 * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Check if context is already cancelled
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	result, err := g.client.Models.GenerateContent(
		ctx,
		g.model,
		genai.Text(prompt),
		nil,
	)

	if err != nil {
		return "", err
	}

	if result == nil {
		return "", fmt.Errorf("no response from LLM")
	}

	return result.Text(), nil
}
