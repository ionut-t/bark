package redact

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"

	"github.com/ionut-t/bark/v2/internal/llm"
)

// Client is an llm.LLM that masks secrets in the system prompt and the
// prompt before passing a call on, and warns about the secrets it masked.
type Client struct {
	next llm.LLM
	warn io.Writer

	mu   sync.Mutex
	seen map[string]bool
}

// Wrap returns client with every call's prompts masked (see Mask). Each
// secret is warned about on warn once, the first time a call holds it: a
// review and its check send the same diff.
func Wrap(client llm.LLM, warn io.Writer) *Client {
	return &Client{next: client, warn: warn, seen: map[string]bool{}}
}

// Generate masks the prompts and makes the call.
func (c *Client) Generate(ctx context.Context, system, prompt string) (llm.Response, error) {
	system, prompt = c.mask(system, prompt)
	return c.next.Generate(ctx, system, prompt)
}

// Stream masks the prompts and starts the call.
func (c *Client) Stream(ctx context.Context, system, prompt string) (<-chan llm.Response, <-chan error) {
	system, prompt = c.mask(system, prompt)
	return c.next.Stream(ctx, system, prompt)
}

func (c *Client) mask(system, prompt string) (string, string) {
	system, prompt, matches := Prompts(system, prompt)
	c.report(matches)
	return system, prompt
}

// Note ends a prompt that had secrets masked. Without it, models take the
// placeholders for text in the code (a string that will break at runtime)
// and copy them into the code they suggest.
const Note = "Note: placeholders such as `[REDACTED:aws-access-key-id]` above stand for secrets that Bark masked before sending this to you. " +
	"The code holds the real secret, not the placeholder: read each one as the hard-coded secret it names, " +
	"and never copy a placeholder into code you write."

// Prompts masks the secrets in a call's system prompt and prompt, as
// Client sends them, and returns what it found. When it masked any, the
// prompt ends with Note; the system prompt only changes where a secret was,
// so it can still be cached.
func Prompts(system, prompt string) (string, string, []Match) {
	system, inSystem := Mask(system)
	prompt, inPrompt := Mask(prompt)
	matches := append(inSystem, inPrompt...)
	if len(matches) > 0 {
		prompt = strings.TrimRight(prompt, "\n") + "\n\n" + Note
	}
	return system, prompt, matches
}

// report warns about the secrets in matches not warned about before.
func (c *Client) report(matches []Match) {
	if len(matches) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var fresh []Match
	for _, m := range matches {
		if c.seen[m.ID] {
			continue
		}
		c.seen[m.ID] = true
		fresh = append(fresh, m)
	}
	if len(fresh) == 0 || c.warn == nil {
		return
	}
	_, _ = fmt.Fprintf(c.warn, "bark: warning: masked %d secret(s) before sending them to the model provider: %s\n",
		len(fresh), describe(fresh))
}

// describe lists the kinds of secrets in matches with their files, each
// pair once, sorted.
func describe(matches []Match) string {
	var items []string
	for _, m := range matches {
		item := m.Kind
		if m.File != "" {
			item += " (" + m.File + ")"
		}
		items = append(items, item)
	}
	slices.Sort(items)
	return strings.Join(slices.Compact(items), ", ")
}
