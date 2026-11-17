package anthropic

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/ionut-t/bark/v2/internal/llm/llmtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var stopReasons = []struct {
	stop      string
	truncated bool
}{
	{"end_turn", false},
	{"stop_sequence", false},
	{"refusal", false},
	{"max_tokens", true},
	{"model_context_window_exceeded", true},
}

// An answer cut off at the output limit is marked as truncated.
func TestGenerateTruncated(t *testing.T) {
	t.Parallel()

	for _, tt := range stopReasons {
		t.Run(tt.stop, func(t *testing.T) {
			t.Parallel()

			body := `{"id":"m","type":"message","role":"assistant","model":"claude-test",` +
				`"content":[{"type":"text","text":"answer"}],"stop_reason":"` + tt.stop + `",` +
				`"usage":{"input_tokens":10,"output_tokens":5}}`
			a := &Anthropic{model: "claude-test", client: anthropic.NewClient(
				option.WithAPIKey("test"),
				option.WithBaseURL("http://llm.test"),
				option.WithHTTPClient(llmtest.JSONClient(body)),
				option.WithMaxRetries(0),
			)}

			resp, err := a.Generate(t.Context(), "", "prompt")
			require.NoError(t, err)
			assert.Equal(t, "answer", resp.Content)
			assert.Equal(t, tt.truncated, resp.Truncated)
		})
	}
}

func TestStreamTruncated(t *testing.T) {
	t.Parallel()

	for _, tt := range stopReasons {
		t.Run(tt.stop, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(
					"event: message_start\n" +
						`data: {"type":"message_start","message":{"id":"m","type":"message","role":"assistant","model":"claude-test","content":[],"stop_reason":null,"usage":{"input_tokens":100,"output_tokens":1}}}` + "\n\n" +
						"event: content_block_start\n" +
						`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
						"event: content_block_delta\n" +
						`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}` + "\n\n" +
						"event: content_block_stop\n" +
						`data: {"type":"content_block_stop","index":0}` + "\n\n" +
						"event: message_delta\n" +
						`data: {"type":"message_delta","delta":{"stop_reason":"` + tt.stop + `"},"usage":{"output_tokens":50}}` + "\n\n" +
						"event: message_stop\n" +
						`data: {"type":"message_stop"}` + "\n\n"))
			}))
			defer srv.Close()

			a := &Anthropic{model: "claude-test", client: anthropic.NewClient(
				option.WithAPIKey("test"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0),
			)}

			out, errc := a.Stream(t.Context(), "", "prompt")
			var text string
			var truncated bool
			for r := range out {
				text += r.Content
				truncated = truncated || r.Truncated
			}
			require.NoError(t, <-errc)
			assert.Equal(t, "hi", text)
			assert.Equal(t, tt.truncated, truncated)
		})
	}
}
