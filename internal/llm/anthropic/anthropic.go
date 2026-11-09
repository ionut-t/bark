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
		var counts usageCounts
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
				u := ev.Message.Usage
				counts.input, counts.cacheRead, counts.cacheWrite = u.InputTokens, u.CacheReadInputTokens, u.CacheCreationInputTokens
				usage = counts.usage()
			case anthropic.MessageDeltaEvent:
				counts.output = ev.Usage.OutputTokens
				// The delta carries up-to-date cumulative input counts when
				// the API sends them; otherwise keep the message_start values.
				if ev.Usage.InputTokens > 0 {
					counts.input = ev.Usage.InputTokens
				}
				if ev.Usage.CacheReadInputTokens > 0 {
					counts.cacheRead = ev.Usage.CacheReadInputTokens
				}
				if ev.Usage.CacheCreationInputTokens > 0 {
					counts.cacheWrite = ev.Usage.CacheCreationInputTokens
				}
				usage = counts.usage()
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
		Usage: usageCounts{
			input:      resp.Usage.InputTokens,
			cacheRead:  resp.Usage.CacheReadInputTokens,
			cacheWrite: resp.Usage.CacheCreationInputTokens,
			output:     resp.Usage.OutputTokens,
		}.usage(),
	}, nil
}

// usageCounts are Anthropic's token counts. Its input count leaves out the
// tokens read from and written to the cache; output includes thinking.
type usageCounts struct {
	input, cacheRead, cacheWrite, output int64
}

func (c usageCounts) usage() *llm.Usage {
	in := c.input + c.cacheRead + c.cacheWrite
	return &llm.Usage{
		InputTokens:       in,
		OutputTokens:      c.output,
		TotalTokens:       in + c.output,
		CachedInputTokens: c.cacheRead,
		CacheWriteTokens:  c.cacheWrite,
	}
}
