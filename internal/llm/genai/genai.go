package genai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ionut-t/bark/v2/internal/llm"
	"google.golang.org/genai"
)

type GenAI struct {
	model string
	// effort is low, medium or high; "" leaves thinking to the model.
	effort string
	client *genai.Client
}

// config builds a request's config, nil when there is nothing to set.
func (g *GenAI) config(system string) *genai.GenerateContentConfig {
	thinking := thinkingConfig(g.model, g.effort)
	if system == "" && thinking == nil {
		return nil
	}
	cfg := &genai.GenerateContentConfig{ThinkingConfig: thinking}
	if system != "" {
		cfg.SystemInstruction = &genai.Content{Parts: []*genai.Part{{Text: system}}}
	}
	return cfg
}

// gemini25Budgets are the thinking budgets for each effort on Gemini 2.5,
// which takes a budget but not a level. 24576 is the most Flash accepts.
var gemini25Budgets = map[string]int32{"low": 1024, "medium": 8192, "high": 24576}

// thinkingConfig maps effort to model's thinking setting: a level on
// Gemini 3 and later, a token budget on Gemini 2.5. Older models don't
// think and may reject either.
func thinkingConfig(model, effort string) *genai.ThinkingConfig {
	if effort == "" {
		return nil
	}
	// The name is the last part of a path: models/..., or a Vertex AI
	// resource name (publishers/google/models/..., projects/.../models/...).
	m := strings.ToLower(model)
	m = m[strings.LastIndex(m, "/")+1:]
	if strings.HasPrefix(m, "gemini-2.5") {
		budget := gemini25Budgets[effort]
		return &genai.ThinkingConfig{ThinkingBudget: &budget}
	}
	return &genai.ThinkingConfig{ThinkingLevel: genai.ThinkingLevel(strings.ToUpper(effort))}
}

func New(ctx context.Context, model, effort string, config genai.ClientConfig) (*GenAI, error) {
	client, err := genai.NewClient(ctx, &config)
	if err != nil {
		return nil, err
	}

	return &GenAI{
		client: client,
		model:  model,
		effort: effort,
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
		stream := g.client.Models.GenerateContentStream(ctx, g.model, contents, g.config(system))

		var usage *llm.Usage
		var truncated bool
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
				usage = usageOf(resp.UsageMetadata)
			}

			if len(resp.Candidates) > 0 {
				truncated = truncated || atLimit(resp)
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

		if usage != nil || truncated {
			select {
			case out <- llm.Response{Usage: usage, Truncated: truncated, Time: time.Now()}:
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

	result, err := g.client.Models.GenerateContent(ctx, g.model, genai.Text(prompt), g.config(system))
	if err != nil {
		return llm.Response{}, fmt.Errorf("genai request failed: %w", err)
	}

	if result == nil {
		return llm.Response{}, fmt.Errorf("no response from LLM")
	}

	return llm.Response{
		Content:   result.Text(),
		Time:      time.Now(),
		Usage:     usageOf(result.UsageMetadata),
		Truncated: atLimit(result),
	}, nil
}

// atLimit says whether the answer stopped because it reached the output
// token limit.
func atLimit(resp *genai.GenerateContentResponse) bool {
	return len(resp.Candidates) > 0 && resp.Candidates[0].FinishReason == genai.FinishReasonMaxTokens
}

// usageOf converts Gemini's usage, or returns nil without one. The prompt
// count includes cached tokens; thinking is in the total, not the output.
func usageOf(md *genai.GenerateContentResponseUsageMetadata) *llm.Usage {
	if md == nil {
		return nil
	}
	return &llm.Usage{
		InputTokens:       int64(md.PromptTokenCount),
		OutputTokens:      int64(md.CandidatesTokenCount),
		TotalTokens:       int64(md.TotalTokenCount),
		CachedInputTokens: int64(md.CachedContentTokenCount),
	}
}
