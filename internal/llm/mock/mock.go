//go:build demo

// Package mock provides an offline LLM that returns a fixed response. It is
// compiled only with the "demo" build tag and is used to record the README
// demo deterministically, without network access or API keys.
package mock

import (
	"context"
	"strings"
	"time"

	"github.com/ionut-t/bark/v2/internal/llm"
)

const (
	thinkDelay = 1200 * time.Millisecond
	chunkDelay = 70 * time.Millisecond
	chunkWords = 2
)

type Mock struct{}

func New() *Mock { return &Mock{} }

func usage() *llm.Usage {
	in, out := int64(1843), int64(len(strings.Fields(review))*4/3)
	return &llm.Usage{InputTokens: in, OutputTokens: out, TotalTokens: in + out}
}

func (m *Mock) Stream(ctx context.Context, system, prompt string) (<-chan llm.Response, <-chan error) {
	out := make(chan llm.Response)
	errChan := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errChan)

		if !sleep(ctx, thinkDelay) {
			errChan <- ctx.Err()
			return
		}

		chunks := split(review)
		for i, c := range chunks {
			resp := llm.Response{Content: c, Time: time.Now()}
			if i == len(chunks)-1 {
				resp.Usage = usage()
			}
			select {
			case out <- resp:
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			}
			if !sleep(ctx, chunkDelay) {
				errChan <- ctx.Err()
				return
			}
		}
	}()

	return out, errChan
}

func (m *Mock) Generate(ctx context.Context, system, prompt string) (llm.Response, error) {
	if !sleep(ctx, thinkDelay) {
		return llm.Response{}, ctx.Err()
	}
	return llm.Response{Content: review, Time: time.Now(), Usage: usage()}, nil
}

func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

// split breaks s into chunks of a few words, keeping whitespace and newlines
// so the concatenated chunks equal s.
func split(s string) []string {
	var chunks []string
	var b strings.Builder
	words := 0
	for i, r := range s {
		b.WriteRune(r)
		if r == ' ' || r == '\n' {
			if i+1 < len(s) && (s[i+1] == ' ' || s[i+1] == '\n') {
				continue
			}
			words++
			if words >= chunkWords {
				chunks = append(chunks, b.String())
				b.Reset()
				words = 0
			}
		}
	}
	if b.Len() > 0 {
		chunks = append(chunks, b.String())
	}
	return chunks
}

const review = "## Verdict: needs work, but it's not garbage\n\n" +
	"Validating input at the boundary is the right call, and wrapping the body in\n" +
	"`http.MaxBytesReader` before decoding is what should have been there from day one.\n" +
	"Good. Now the parts that are wrong.\n\n" +
	"### 1. `len()` counts bytes, not characters\n\n" +
	"```go\nif len(req.Name) > 100 {\n```\n\n" +
	"Your error message says \"100 characters\". `len` returns bytes. A name in Cyrillic\n" +
	"hits the limit at half that. Use `utf8.RuneCountInString`, or say \"bytes\".\n" +
	"Don't lie in error messages.\n\n" +
	"### 2. `mail.ParseAddress` accepts more than you think\n\n" +
	"`\"Bob <bob@example.com>\"` parses just fine, and then you store the whole string\n" +
	"as the email. Use the parsed result:\n\n" +
	"```go\naddr, err := mail.ParseAddress(req.Email)\nif err != nil || addr.Address != req.Email {\n```\n\n" +
	"**Would I merge it?** After fixing 1 and 2. They're five-line changes. Send a v2.\n"
