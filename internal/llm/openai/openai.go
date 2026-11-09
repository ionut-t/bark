package openai

import (
	"context"
	"fmt"
	"time"

	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

type OpenAI struct {
	model  string
	client openai.Client
}

func applySystem(params *responses.ResponseNewParams, system string) {
	if system != "" {
		params.Instructions = openai.String(system)
	}
}

func New(model string, apiKey string) *OpenAI {
	client := openai.NewClient(option.WithAPIKey(apiKey))
	return &OpenAI{
		model:  model,
		client: client,
	}
}

func (o *OpenAI) Stream(ctx context.Context, system, prompt string) (<-chan llm.Response, <-chan error) {
	out := make(chan llm.Response)
	errChan := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errChan)

		if ctx.Err() != nil {
			errChan <- ctx.Err()
			return
		}

		params := responses.ResponseNewParams{
			Input: responses.ResponseNewParamsInputUnion{OfString: openai.String(prompt)},
			Model: openai.ChatModel(o.model),
		}
		applySystem(&params, system)

		stream := o.client.Responses.NewStreaming(ctx, params)
		// Runs before close(errChan). The send must not block: errChan may
		// already hold an earlier error that nobody reads, so the first wins.
		defer func() {
			if err := stream.Close(); err != nil {
				select {
				case errChan <- fmt.Errorf("close stream: %w", err):
				default:
				}
			}
		}()

		var usage *llm.Usage
		for stream.Next() {
			select {
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			default:
			}

			event := stream.Current()
			if event.Type == "response.completed" {
				usage = usageOf(event.AsResponseCompleted().Response.Usage)
				continue
			}

			if event.Type != "response.output_text.delta" {
				continue
			}

			delta := event.AsResponseOutputTextDelta()
			if delta.Delta == "" {
				continue
			}

			select {
			case out <- llm.Response{
				Content: delta.Delta,
				Time:    time.Now(),
			}:
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			}
		}

		if err := stream.Err(); err != nil {
			errChan <- err
		} else if usage != nil {
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

func (o *OpenAI) Generate(ctx context.Context, system, prompt string) (llm.Response, error) {
	if ctx.Err() != nil {
		return llm.Response{}, ctx.Err()
	}

	params := responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{OfString: openai.String(prompt)},
		Model: openai.ChatModel(o.model),
	}
	applySystem(&params, system)

	resp, err := o.client.Responses.New(ctx, params)
	if err != nil {
		return llm.Response{}, fmt.Errorf("openai request failed: %w", err)
	}

	text := resp.OutputText()
	if text == "" {
		return llm.Response{}, fmt.Errorf("no response from LLM")
	}

	return llm.Response{
		Content: text,
		Time:    time.Now(),
		Usage:   usageOf(resp.Usage),
	}, nil
}

// usageOf converts OpenAI's usage. Its input count includes cached tokens,
// and its output count includes reasoning.
func usageOf(u responses.ResponseUsage) *llm.Usage {
	return &llm.Usage{
		InputTokens:       u.InputTokens,
		OutputTokens:      u.OutputTokens,
		TotalTokens:       u.TotalTokens,
		CachedInputTokens: u.InputTokensDetails.CachedTokens,
		CacheWriteTokens:  u.InputTokensDetails.CacheWriteTokens,
	}
}
