package anthropic

import (
	"context"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/ionut-t/bark/v2/internal/llm"
)

const maxTokens = 16000

type Anthropic struct {
	model  string
	client anthropic.Client
}

func applySystem(params *anthropic.MessageNewParams, system string) {
	if system != "" {
		params.System = []anthropic.TextBlockParam{{Text: system}}
	}
}

func New(model string, apiKey string) *Anthropic {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &Anthropic{
		model:  model,
		client: client,
	}
}

func (a *Anthropic) Stream(ctx context.Context, system, prompt string) (<-chan llm.Response, <-chan error) {
	out := make(chan llm.Response)
	errChan := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errChan)

		if ctx.Err() != nil {
			errChan <- ctx.Err()
			return
		}

		params := anthropic.MessageNewParams{
			Model:     anthropic.Model(a.model),
			MaxTokens: maxTokens,
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
			},
		}
		applySystem(&params, system)

		stream := a.client.Messages.NewStreaming(ctx, params)

		defer func() {
			if err := stream.Close(); err != nil {
				errChan <- err
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
			switch ev := event.AsAny().(type) {
			case anthropic.MessageStartEvent:
				if usage == nil {
					usage = &llm.Usage{}
				}
				usage.InputTokens = ev.Message.Usage.InputTokens
				usage.TotalTokens = usage.InputTokens + usage.OutputTokens
			case anthropic.MessageDeltaEvent:
				if usage == nil {
					usage = &llm.Usage{}
				}
				usage.OutputTokens = ev.Usage.OutputTokens
				// The delta carries the up-to-date cumulative input count when
				// the API sends it; otherwise keep the message_start value.
				if ev.Usage.InputTokens > 0 {
					usage.InputTokens = ev.Usage.InputTokens
				}
				usage.TotalTokens = usage.InputTokens + usage.OutputTokens
			case anthropic.ContentBlockDeltaEvent:
				textDelta, ok := ev.Delta.AsAny().(anthropic.TextDelta)
				if !ok || textDelta.Text == "" {
					continue
				}

				select {
				case out <- llm.Response{Content: textDelta.Text, Time: time.Now()}:
				case <-ctx.Done():
					errChan <- ctx.Err()
					return
				}
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

func (a *Anthropic) Generate(ctx context.Context, system, prompt string) (llm.Response, error) {
	if ctx.Err() != nil {
		return llm.Response{}, ctx.Err()
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: maxTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	}
	applySystem(&params, system)

	resp, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return llm.Response{}, fmt.Errorf("anthropic request failed: %w", err)
	}

	if len(resp.Content) == 0 {
		return llm.Response{}, fmt.Errorf("no response from anthropic")
	}

	text, ok := resp.Content[0].AsAny().(anthropic.TextBlock)
	if !ok || text.Text == "" {
		return llm.Response{}, fmt.Errorf("empty response from anthropic")
	}

	return llm.Response{
		Content: text.Text,
		Time:    time.Now(),
		Usage: &llm.Usage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
			TotalTokens:  resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}, nil
}
