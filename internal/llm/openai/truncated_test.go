package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/ionut-t/bark/v2/internal/llm/llmtest"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const usageJSON = `"usage":{"input_tokens":10,"input_tokens_details":{"cached_tokens":0},"output_tokens":5,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":15}`

var endings = []struct {
	name      string
	status    string
	details   string
	event     string
	truncated bool
}{
	{name: "completed", status: "completed", details: `null`, event: "response.completed"},
	{name: "output limit", status: "incomplete", details: `{"reason":"max_output_tokens"}`, event: "response.incomplete", truncated: true},
	{name: "content filter", status: "incomplete", details: `{"reason":"content_filter"}`, event: "response.incomplete"},
}

func responseJSON(status, details string) string {
	return `{"id":"r","object":"response","created_at":0,"model":"gpt-test","status":"` + status + `",` +
		`"incomplete_details":` + details + `,` +
		`"output":[{"type":"message","id":"m","role":"assistant","status":"` + status + `",` +
		`"content":[{"type":"output_text","text":"answer","annotations":[]}]}],` + usageJSON + `}`
}

// An answer cut off at the output limit is marked as truncated.
func TestGenerateTruncated(t *testing.T) {
	t.Parallel()

	for _, tt := range endings {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			o := &OpenAI{model: "gpt-test", client: openai.NewClient(
				option.WithAPIKey("test"),
				option.WithBaseURL("http://llm.test"),
				option.WithHTTPClient(llmtest.JSONClient(responseJSON(tt.status, tt.details))),
				option.WithMaxRetries(0),
			)}

			resp, err := o.Generate(t.Context(), "", "prompt")
			require.NoError(t, err)
			assert.Equal(t, "answer", resp.Content)
			assert.Equal(t, tt.truncated, resp.Truncated)
		})
	}
}

// A stream that ends incomplete is marked as truncated when it hit the
// output limit, and still reports its usage.
func TestStreamTruncated(t *testing.T) {
	t.Parallel()

	for _, tt := range endings {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(
					"event: response.output_text.delta\n" +
						`data: {"type":"response.output_text.delta","item_id":"m","output_index":0,"content_index":0,"delta":"answer","logprobs":[],"sequence_number":1}` + "\n\n" +
						"event: " + tt.event + "\n" +
						`data: {"type":"` + tt.event + `","sequence_number":2,"response":` + responseJSON(tt.status, tt.details) + `}` + "\n\n"))
			}))
			defer srv.Close()

			o := &OpenAI{model: "gpt-test", client: openai.NewClient(
				option.WithAPIKey("test"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0),
			)}

			out, errc := o.Stream(t.Context(), "", "prompt")
			var text string
			var usage *llm.Usage
			var truncated bool
			for r := range out {
				text += r.Content
				truncated = truncated || r.Truncated
				if r.Usage != nil {
					usage = r.Usage
				}
			}
			require.NoError(t, <-errc)
			assert.Equal(t, "answer", text)
			assert.Equal(t, tt.truncated, truncated)
			require.NotNil(t, usage)
			assert.Equal(t, int64(15), usage.TotalTokens)
		})
	}
}
