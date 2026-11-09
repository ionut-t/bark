// Package ollama talks to a local Ollama server through its native chat API.
//
// Ollama's OpenAI-compatible endpoint can't set the context size, and a
// prompt longer than the context the server loads the model with (4096
// tokens by default, more on some machines or with OLLAMA_CONTEXT_LENGTH)
// is cut without an error: a 35k-token check prompt was cut to 16k, lost
// its instructions, and the model wrote a review instead of verdicts. The native API takes the context size per request, so each request
// asks for one that fits its prompt.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ionut-t/bark/v2/internal/llm"
)

type Ollama struct {
	model  string
	host   string
	client *http.Client

	showOnce   sync.Once
	maxContext int // the model's context length; 0 if unknown
}

func New(model string) *Ollama {
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = "http://localhost:11434"
	}
	return &Ollama{model: model, host: strings.TrimRight(host, "/"), client: http.DefaultClient}
}

// Sizing the context: tokens are estimated from the prompt's length, with
// room for the answer. Code and JSON tokenise densely (2.6 characters per
// token for a Go check prompt on gemma3), so the estimate errs high. The
// size is rounded up to a power of two so that consecutive requests usually
// ask for the same one; Ollama reloads the model when it changes.
const (
	charsPerToken = 2.5
	outputReserve = 8192
)

// numCtx returns the context size for a request. A prompt that can't fit in
// the model's context is an error rather than being cut.
func (o *Ollama) numCtx(ctx context.Context, system, prompt string) (int, error) {
	need := int(float64(len(system)+len(prompt))/charsPerToken) + outputReserve
	n := outputReserve
	for n < need {
		n *= 2
	}
	if limit := o.contextLength(ctx); limit > 0 {
		if need > limit {
			return 0, fmt.Errorf("the prompt is about %d tokens, more than %s's context of %d tokens: review a smaller change (--max-diff-lines) or use a model with a longer context",
				need-outputReserve, o.model, limit)
		}
		n = min(n, limit)
	}
	return n, nil
}

// contextLength asks the server once for the model's context length. It is
// 0 if the server doesn't say.
func (o *Ollama) contextLength(ctx context.Context) int {
	o.showOnce.Do(func() {
		body, _ := json.Marshal(map[string]string{"model": o.model})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.host+"/api/show", bytes.NewReader(body))
		if err != nil {
			return
		}
		resp, err := o.client.Do(req)
		if err != nil {
			return
		}
		defer func() { _ = resp.Body.Close() }()
		var show struct {
			ModelInfo map[string]any `json:"model_info"`
		}
		if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&show) != nil {
			return
		}
		arch, _ := show.ModelInfo["general.architecture"].(string)
		if n, ok := show.ModelInfo[arch+".context_length"].(float64); ok {
			o.maxContext = int(n)
		}
	})
	return o.maxContext
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string         `json:"model"`
	Messages []message      `json:"messages"`
	Stream   bool           `json:"stream"`
	Options  map[string]any `json:"options"`
}

// chatChunk is one line of a chat response: a piece of the answer, the
// final line with the token counts, or an error.
type chatChunk struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done            bool   `json:"done"`
	PromptEvalCount int64  `json:"prompt_eval_count"`
	EvalCount       int64  `json:"eval_count"`
	Error           string `json:"error"`
}

func (c chatChunk) usage() *llm.Usage {
	if c.PromptEvalCount+c.EvalCount == 0 {
		return nil
	}
	return &llm.Usage{InputTokens: c.PromptEvalCount, OutputTokens: c.EvalCount, TotalTokens: c.PromptEvalCount + c.EvalCount}
}

// chat sends a chat request and returns the response body.
func (o *Ollama) chat(ctx context.Context, system, prompt string, stream bool) (io.ReadCloser, error) {
	numCtx, err := o.numCtx(ctx, system, prompt)
	if err != nil {
		return nil, err
	}
	var msgs []message
	if system != "" {
		msgs = append(msgs, message{Role: "system", Content: system})
	}
	msgs = append(msgs, message{Role: "user", Content: prompt})
	body, err := json.Marshal(chatRequest{Model: o.model, Messages: msgs, Stream: stream, Options: map[string]any{"num_ctx": numCtx}})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.host+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		var e chatChunk
		data, _ := io.ReadAll(resp.Body)
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return nil, fmt.Errorf("ollama request failed: %s: %s", resp.Status, e.Error)
		}
		return nil, fmt.Errorf("ollama request failed: %s", resp.Status)
	}
	return resp.Body, nil
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

		body, err := o.chat(ctx, system, prompt, true)
		if err != nil {
			errChan <- err
			return
		}
		// Runs before close(errChan). The send must not block: errChan may
		// already hold an earlier error that nobody reads, so the first wins.
		defer func() {
			if err := body.Close(); err != nil {
				select {
				case errChan <- fmt.Errorf("close stream: %w", err):
				default:
				}
			}
		}()

		send := func(r llm.Response) bool {
			select {
			case out <- r:
				return true
			case <-ctx.Done():
				errChan <- ctx.Err()
				return false
			}
		}

		dec := json.NewDecoder(body)
		for {
			var chunk chatChunk
			if err := dec.Decode(&chunk); err != nil {
				switch {
				case ctx.Err() != nil:
					errChan <- ctx.Err()
				case errors.Is(err, io.EOF):
					errChan <- errors.New("ollama stream ended early")
				default:
					errChan <- fmt.Errorf("ollama stream: %w", err)
				}
				return
			}
			if chunk.Error != "" {
				errChan <- fmt.Errorf("ollama stream: %s", chunk.Error)
				return
			}
			if c := chunk.Message.Content; c != "" && !send(llm.Response{Content: c, Time: time.Now()}) {
				return
			}
			if chunk.Done {
				if u := chunk.usage(); u != nil {
					send(llm.Response{Usage: u, Time: time.Now()})
				}
				return
			}
		}
	}()

	return out, errChan
}

func (o *Ollama) Generate(ctx context.Context, system, prompt string) (llm.Response, error) {
	if ctx.Err() != nil {
		return llm.Response{}, ctx.Err()
	}

	body, err := o.chat(ctx, system, prompt, false)
	if err != nil {
		return llm.Response{}, err
	}
	defer func() { _ = body.Close() }()

	var resp chatChunk
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return llm.Response{}, fmt.Errorf("ollama response: %w", err)
	}
	if resp.Error != "" {
		return llm.Response{}, fmt.Errorf("ollama request failed: %s", resp.Error)
	}
	if resp.Message.Content == "" {
		return llm.Response{}, fmt.Errorf("empty response from Ollama")
	}

	return llm.Response{
		Content: resp.Message.Content,
		Time:    time.Now(),
		Usage:   resp.usage(),
	}, nil
}
