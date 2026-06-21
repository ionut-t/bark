package ollama

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ionut-t/bark/v2/pkg/llm"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type Ollama struct {
	model  string
	client openai.Client
}

func New(model string) *Ollama {
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = "http://localhost:11434"
	}
	host = strings.TrimRight(host, "/")

	client := openai.NewClient(
		option.WithBaseURL(host+"/v1"),
		option.WithAPIKey("ollama"),
	)

	return &Ollama{model: model, client: client}
}

func (o *Ollama) Stream(ctx context.Context, system, prompt string) (<-chan llm.Response, <-chan error) {
	out := make(chan llm.Response)
	errChan := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errChan)

		if ctx.Err() != nil {
			errChan <- ctx.Err()
			return
		}

		messages := []openai.ChatCompletionMessageParamUnion{}
		if system != "" {
			messages = append(messages, openai.SystemMessage(system))
		}
		messages = append(messages, openai.UserMessage(prompt))

		stream := o.client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
			Model:    openai.ChatModel(o.model),
			Messages: messages,
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

			chunk := stream.Current()
			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0].Delta.Content
			if delta == "" {
				continue
			}

			select {
			case out <- llm.Response{Content: delta, Time: time.Now()}:
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

func (o *Ollama) Generate(ctx context.Context, system, prompt string) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	messages := []openai.ChatCompletionMessageParamUnion{}
	if system != "" {
		messages = append(messages, openai.SystemMessage(system))
	}
	messages = append(messages, openai.UserMessage(prompt))

	resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(o.model),
		Messages: messages,
	})
	if err != nil {
		return "", fmt.Errorf("Ollama request failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from Ollama")
	}

	content := resp.Choices[0].Message.Content
	if content == "" {
		return "", fmt.Errorf("empty response from Ollama")
	}

	return content, nil
}
