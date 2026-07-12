package genai

import (
	"context"
	"fmt"
	"time"

	"github.com/ionut-t/bark/v2/internal/llm"
	"google.golang.org/genai"
)

type GenAI struct {
	model  string
	client *genai.Client
}

func systemConfig(system string) *genai.GenerateContentConfig {
	if system == "" {
		return nil
	}
	return &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{Parts: []*genai.Part{{Text: system}}},
	}
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

func (g *GenAI) Stream(ctx context.Context, system, prompt string) (<-chan llm.Response, <-chan error) {
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
		stream := g.client.Models.GenerateContentStream(ctx, g.model, contents, systemConfig(system))

		var usage *llm.Usage
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

			if resp.UsageMetadata != nil {
				usage = &llm.Usage{
					InputTokens:  int64(resp.UsageMetadata.PromptTokenCount),
					OutputTokens: int64(resp.UsageMetadata.CandidatesTokenCount),
					TotalTokens:  int64(resp.UsageMetadata.TotalTokenCount),
				}
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

		if usage != nil {
			select {
			case out <- llm.Response{Usage: usage, Time: time.Now()}:
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			}
		}
	}()

	return out, errChan
}

func (g *GenAI) Generate(ctx context.Context, system, prompt string) (llm.Response, error) {
	if ctx.Err() != nil {
		return llm.Response{}, ctx.Err()
	}

	result, err := g.client.Models.GenerateContent(ctx, g.model, genai.Text(prompt), systemConfig(system))
	if err != nil {
		return llm.Response{}, fmt.Errorf("genai request failed: %w", err)
	}

	if result == nil {
		return llm.Response{}, fmt.Errorf("no response from LLM")
	}

	var usage *llm.Usage
	if result.UsageMetadata != nil {
		usage = &llm.Usage{
			InputTokens:  int64(result.UsageMetadata.PromptTokenCount),
			OutputTokens: int64(result.UsageMetadata.CandidatesTokenCount),
			TotalTokens:  int64(result.UsageMetadata.TotalTokenCount),
		}
	}

	return llm.Response{
		Content: result.Text(),
		Time:    time.Now(),
		Usage:   usage,
	}, nil
}
