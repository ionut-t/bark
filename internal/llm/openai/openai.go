package openai

import (
	"context"
	"fmt"
	"time"

	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

type OpenAI struct {
	model string
	// effort is low, medium or high; "" leaves reasoning to the model.
	effort string
	client openai.Client
}

func applySystem(params *responses.ResponseNewParams, system string) {
	if system != "" {
		params.Instructions = openai.String(system)
	}
}

// params builds a request. Only reasoning models (gpt-5, o-series) accept
// an effort; others may reject the request.
func (o *OpenAI) params(system, prompt string) responses.ResponseNewParams {
	params := responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{OfString: openai.String(prompt)},
		Model: openai.ChatModel(o.model),
	}
	applySystem(&params, system)
	if o.effort != "" {
		params.Reasoning = shared.ReasoningParam{Effort: shared.ReasoningEffort(o.effort)}
	}
	return params
}

func New(model, apiKey, effort string) *OpenAI {
	client := openai.NewClient(option.WithAPIKey(apiKey))
	return &OpenAI{
		model:  model,
		effort: effort,
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

		params := o.params(system, prompt)

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
		var truncated bool
		for stream.Next() {
			select {
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			default:
			}

			event := stream.Current()
			switch event.Type {
			case "response.completed":
				usage = usageOf(event.AsResponseCompleted().Response.Usage)
				continue
			case "response.incomplete":
				resp := event.AsResponseIncomplete().Response
				usage = usageOf(resp.Usage)
				truncated = atLimit(resp)
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
		} else if usage != nil || truncated {
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

func (o *OpenAI) Generate(ctx context.Context, system, prompt string) (llm.Response, error) {
	if ctx.Err() != nil {
		return llm.Response{}, ctx.Err()
	}

	params := o.params(system, prompt)

	resp, err := o.client.Responses.New(ctx, params)
	if err != nil {
		return llm.Response{}, fmt.Errorf("openai request failed: %w", err)
	}

	text := resp.OutputText()
	if text == "" {
		return llm.Response{}, fmt.Errorf("no response from LLM")
	}

	return llm.Response{
		Content:   text,
		Time:      time.Now(),
		Usage:     usageOf(resp.Usage),
		Truncated: atLimit(*resp),
	}, nil
}

// atLimit says whether the response stopped because it reached the output
// token limit.
func atLimit(r responses.Response) bool {
	return r.Status == responses.ResponseStatusIncomplete && r.IncompleteDetails.Reason == "max_output_tokens"
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
