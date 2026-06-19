package anthropic

import (
	"context"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/ionut-t/bark/v2/pkg/llm"
)

type Anthropic struct {
	model  string
	client anthropic.Client
}

func New(model string, apiKey string) *Anthropic {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &Anthropic{
		model:  model,
		client: client,
	}
}

func (a *Anthropic) Stream(ctx context.Context, prompt string) (<-chan llm.Response, <-chan error) {
	out := make(chan llm.Response)
	errChan := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errChan)

		if ctx.Err() != nil {
			errChan <- ctx.Err()
			return
		}

		stream := a.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
			Model:     anthropic.Model(a.model),
			MaxTokens: 16000,
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
			},
		})

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
			delta, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent)
			if !ok {
				continue
			}

			textDelta, ok := delta.Delta.AsAny().(anthropic.TextDelta)
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

		if err := stream.Err(); err != nil {
			errChan <- err
		}
	}()

	return out, errChan
}

func (a *Anthropic) Generate(ctx context.Context, prompt string) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: 16000,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("Anthropic request failed: %w", err)
	}

	if len(resp.Content) == 0 {
		return "", fmt.Errorf("no response from Anthropic")
	}

	text, ok := resp.Content[0].AsAny().(anthropic.TextBlock)
	if !ok || text.Text == "" {
		return "", fmt.Errorf("empty response from Anthropic")
	}

	return text.Text, nil
}
