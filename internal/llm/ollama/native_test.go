package ollama

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeOllama is an Ollama server that records the context size each chat
// request asks for.
type fakeOllama struct {
	contextLength int // reported by /api/show; 0 makes /api/show fail
	chat          func(w http.ResponseWriter, stream bool)

	mu     sync.Mutex
	numCtx []int
	paths  []string
}

func (f *fakeOllama) serve(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.paths = append(f.paths, r.URL.Path)
		f.mu.Unlock()
		switch r.URL.Path {
		case "/api/show":
			if f.contextLength == 0 {
				http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
				return
			}
			_, _ = fmt.Fprintf(w, `{"model_info":{"general.architecture":"gemma3","gemma3.context_length":%d}}`, f.contextLength)
		case "/api/chat":
			var req struct {
				Stream  bool `json:"stream"`
				Options struct {
					NumCtx int `json:"num_ctx"`
				} `json:"options"`
			}
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			f.mu.Lock()
			f.numCtx = append(f.numCtx, req.Options.NumCtx)
			f.mu.Unlock()
			f.chat(w, req.Stream)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("OLLAMA_HOST", srv.URL)
}

func reply(w http.ResponseWriter, stream bool) {
	if stream {
		_, _ = fmt.Fprintln(w, `{"message":{"role":"assistant","content":"Hel"},"done":false}`)
		_, _ = fmt.Fprintln(w, `{"message":{"role":"assistant","content":"lo"},"done":false}`)
		_, _ = fmt.Fprintln(w, `{"message":{"role":"assistant","content":""},"done":true,"prompt_eval_count":40000,"eval_count":2}`)
		return
	}
	_, _ = fmt.Fprintln(w, `{"message":{"role":"assistant","content":"Hello"},"done":true,"prompt_eval_count":40000,"eval_count":2}`)
}

func collect(t *testing.T, out <-chan llm.Response, errc <-chan error) (string, *llm.Usage, error) {
	t.Helper()
	var b strings.Builder
	var usage *llm.Usage
	for r := range out {
		b.WriteString(r.Content)
		if r.Usage != nil {
			usage = r.Usage
		}
	}
	return b.String(), usage, <-errc
}

// A long prompt must fit in the context the request asks for; Ollama's
// default (4096 tokens, or whatever the server was started with) cuts it
// without an error.
func TestContextFitsThePrompt(t *testing.T) {
	prompt := strings.Repeat("x", 100_000) // ~40k tokens at 2.5 characters each
	want := &llm.Usage{InputTokens: 40000, OutputTokens: 2, TotalTokens: 40002}

	t.Run("generate", func(t *testing.T) {
		f := &fakeOllama{contextLength: 131072, chat: reply}
		f.serve(t)

		resp, err := New("gemma3:12b").Generate(t.Context(), "system", prompt)
		require.NoError(t, err)
		assert.Equal(t, "Hello", resp.Content)
		assert.Equal(t, want, resp.Usage)
		assert.Equal(t, []int{65536}, f.numCtx)
	})

	t.Run("stream", func(t *testing.T) {
		f := &fakeOllama{contextLength: 131072, chat: reply}
		f.serve(t)

		out, errc := New("gemma3:12b").Stream(t.Context(), "system", prompt)
		content, usage, err := collect(t, out, errc)
		require.NoError(t, err)
		assert.Equal(t, "Hello", content)
		assert.Equal(t, want, usage)
		assert.Equal(t, []int{65536}, f.numCtx)
	})
}

func TestContextSize(t *testing.T) {
	tests := []struct {
		name          string
		promptChars   int
		contextLength int
		want          int
	}{
		{"a short prompt still has room for the answer", 100, 131072, 16384},
		{"rounded up to a power of two", 30_000, 131072, 32768},    // 12k + 8k reserve
		{"capped at the model's context", 250_000, 131072, 131072}, // 100k + 8k
		{"unknown model context", 250_000, 0, 131072},
		// Small-context models: the answer gets what the prompt leaves
		// (PR #107 review: a fixed 8k reserve rejected every prompt).
		{"a 4k model with a short prompt", 1_000, 4096, 4096},
		{"an 8k model with a 4k-token prompt", 10_000, 8192, 8192},
		{"a 2k model with a short prompt", 500, 2048, 2048},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeOllama{contextLength: tt.contextLength, chat: reply}
			f.serve(t)

			_, err := New("m").Generate(t.Context(), "", strings.Repeat("x", tt.promptChars))
			require.NoError(t, err)
			assert.Equal(t, []int{tt.want}, f.numCtx)
		})
	}
}

func TestPromptLongerThanTheModelContext(t *testing.T) {
	f := &fakeOllama{contextLength: 8192, chat: reply}
	f.serve(t)
	o := New("small")

	_, err := o.Generate(t.Context(), "", strings.Repeat("x", 100_000))
	require.ErrorContains(t, err, "too long for small's context of 8192 tokens")

	// A prompt that fits but leaves too little room for an answer.
	_, err = o.Generate(t.Context(), "", strings.Repeat("x", 19_000)) // ~7.6k tokens
	require.ErrorContains(t, err, "context of 8192 tokens")

	out, errc := o.Stream(t.Context(), "", strings.Repeat("x", 100_000))
	_, _, err = collect(t, out, errc)
	require.ErrorContains(t, err, "too long for small's context")

	assert.Equal(t, []string{"/api/show"}, f.paths, "no chat request; the model info is asked for once")
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name string
		chat func(w http.ResponseWriter, stream bool)
		want string
	}{
		{"HTTP error", func(w http.ResponseWriter, _ bool) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"error":"model 'm' not found"}`)
		}, "404 Not Found: model 'm' not found"},
		{"error line", func(w http.ResponseWriter, _ bool) {
			_, _ = fmt.Fprintln(w, `{"error":"out of memory"}`)
		}, "out of memory"},
		{"empty answer", func(w http.ResponseWriter, stream bool) {
			_, _ = fmt.Fprintln(w, `{"message":{"content":""},"done":true}`)
		}, ""},
		{"cut off", func(w http.ResponseWriter, stream bool) {
			if stream {
				_, _ = fmt.Fprintln(w, `{"message":{"content":"Hel"},"done":false}`)
			}
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeOllama{contextLength: 131072, chat: tt.chat}
			f.serve(t)
			o := New("m")

			_, genErr := o.Generate(t.Context(), "", "p")
			out, errc := o.Stream(t.Context(), "", "p")
			_, _, streamErr := collect(t, out, errc)

			switch tt.name {
			case "empty answer":
				require.ErrorContains(t, genErr, "empty response")
				require.NoError(t, streamErr)
			case "cut off":
				require.Error(t, genErr)
				require.ErrorContains(t, streamErr, "ended early")
			default:
				require.ErrorContains(t, genErr, tt.want)
				require.ErrorContains(t, streamErr, tt.want)
			}
		})
	}
}
