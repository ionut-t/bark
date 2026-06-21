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
		defer func() {
			if err := stream.Close(); err != nil {
				errChan <- err
			}
		}()

		for stream.Next() {
			select {
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			default:
			}

			event := stream.Current()
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
		}
	}()

	return out, errChan
}

func (o *OpenAI) Generate(ctx context.Context, system, prompt string) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	params := responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{OfString: openai.String(prompt)},
		Model: openai.ChatModel(o.model),
	}
	applySystem(&params, system)

	resp, err := o.client.Responses.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("openai request failed: %w", err)
	}

	text := resp.OutputText()
	if text == "" {
		return "", fmt.Errorf("no response from LLM")
	}

	return text, nil
}
