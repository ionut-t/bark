package genai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func newTestClient(t *testing.T, finishReason string) *GenAI {
	t.Helper()
	chunk := func(text, finish string) string {
		s := `{"candidates":[{"content":{"role":"model","parts":[{"text":"` + text + `"}]}`
		if finish != "" {
			s += `,"finishReason":"` + finish + `"`
		}
		return s + `}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "streamGenerateContent") {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: " + chunk("ans", "") + "\n\n" + "data: " + chunk("wer", finishReason) + "\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chunk("answer", finishReason)))
	}))
	t.Cleanup(srv.Close)

	g, err := New(t.Context(), "gemini-test", "", genai.ClientConfig{
		APIKey:      "test",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: srv.URL},
	})
	require.NoError(t, err)
	return g
}

// An answer cut off at the output limit is marked as truncated.
func TestTruncated(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		finish    string
		truncated bool
	}{
		{"STOP", false},
		{"SAFETY", false},
		{"MAX_TOKENS", true},
	} {
		t.Run(tt.finish, func(t *testing.T) {
			t.Parallel()
			g := newTestClient(t, tt.finish)

			resp, err := g.Generate(t.Context(), "", "prompt")
			require.NoError(t, err)
			assert.Equal(t, "answer", resp.Content)
			assert.Equal(t, tt.truncated, resp.Truncated)

			out, errc := g.Stream(t.Context(), "", "prompt")
			var text string
			var truncated bool
			for r := range out {
				text += r.Content
				truncated = truncated || r.Truncated
			}
			require.NoError(t, <-errc)
			assert.Equal(t, "answer", text)
			assert.Equal(t, tt.truncated, truncated)
		})
	}
}
