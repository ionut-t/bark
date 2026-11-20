package anthropic

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/ionut-t/bark/v2/internal/llm"
)

const maxTokens = 16000

type Anthropic struct {
	model string
	// effort is low, medium or high; "" leaves thinking to the model.
	effort string
	client anthropic.Client
}

func applySystem(params *anthropic.MessageNewParams, system string) {
	if system != "" {
		params.System = []anthropic.TextBlockParam{{Text: system}}
	}
}

// params builds a request. An effort also turns on adaptive thinking: some
// models (Opus 4.6-4.8) don't think unless asked, and effort alone would
// leave them answering without it.
func (a *Anthropic) params(system, prompt string) anthropic.MessageNewParams {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: maxTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	}
	applySystem(&params, system)
	if a.effort != "" {
		params.OutputConfig = anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffort(a.effort)}
		params.Thinking = anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}}
	}
	return params
}

func New(model, apiKey, effort string) *Anthropic {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &Anthropic{
		model:  model,
		effort: effort,
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

		params := a.params(system, prompt)

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
		var truncated bool
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
				// There can be more than one delta; a later one without a
				// stop reason doesn't undo the cut.
				truncated = truncated || atLimit(ev.Delta.StopReason)
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

func (a *Anthropic) Generate(ctx context.Context, system, prompt string) (llm.Response, error) {
	if ctx.Err() != nil {
		return llm.Response{}, ctx.Err()
	}

	params := a.params(system, prompt)

	resp, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return llm.Response{}, fmt.Errorf("anthropic request failed: %w", err)
	}

	// Thinking blocks can come before the text, and the text can be split
	// across several blocks, so join every text block.
	var text strings.Builder
	types := make([]string, 0, len(resp.Content))
	for _, block := range resp.Content {
		types = append(types, block.Type)
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(tb.Text)
		}
	}
	if text.Len() == 0 {
		blocks := "none"
		if len(types) > 0 {
			blocks = strings.Join(types, ", ")
		}
		return llm.Response{}, fmt.Errorf("no text in anthropic response (content blocks: %s; stop reason: %s)", blocks, resp.StopReason)
	}

	return llm.Response{
		Content: text.String(),
		Time:    time.Now(),
		Usage: usageCounts{
			input:      resp.Usage.InputTokens,
			cacheRead:  resp.Usage.CacheReadInputTokens,
			cacheWrite: resp.Usage.CacheCreationInputTokens,
			output:     resp.Usage.OutputTokens,
		}.usage(),
		Truncated: atLimit(resp.StopReason),
	}, nil
}

// atLimit says whether the model stopped because it ran out of room for
// its answer.
func atLimit(r anthropic.StopReason) bool {
	return r == anthropic.StopReasonMaxTokens || r == anthropic.StopReasonModelContextWindowExceeded
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
