package anthropic

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Anthropic leaves the tokens read from and written to the cache out of its
// input count; Bark's input count includes them.
func TestStreamUsageIncludesCachedInput(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			"event: message_start\n" +
				`data: {"type":"message_start","message":{"id":"m","type":"message","role":"assistant","model":"claude-test","content":[],"stop_reason":null,"usage":{"input_tokens":100,"cache_read_input_tokens":3000,"cache_creation_input_tokens":200,"output_tokens":1}}}` + "\n\n" +
				"event: content_block_start\n" +
				`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
				"event: content_block_delta\n" +
				`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}` + "\n\n" +
				"event: content_block_stop\n" +
				`data: {"type":"content_block_stop","index":0}` + "\n\n" +
				"event: message_delta\n" +
				`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":50}}` + "\n\n" +
				"event: message_stop\n" +
				`data: {"type":"message_stop"}` + "\n\n"))
	}))
	defer srv.Close()

	a := &Anthropic{model: "claude-test", client: anthropic.NewClient(
		option.WithAPIKey("test"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0),
	)}

	out, errc := a.Stream(t.Context(), "", "prompt")
	var usage *llm.Usage
	var text string
	for r := range out {
		text += r.Content
		if r.Usage != nil {
			usage = r.Usage
		}
	}
	require.NoError(t, <-errc)
	assert.Equal(t, "hi", text)
	require.NotNil(t, usage)
	assert.Equal(t, llm.Usage{
		InputTokens:       3_300,
		OutputTokens:      50,
		TotalTokens:       3_350,
		CachedInputTokens: 3_000,
		CacheWriteTokens:  200,
	}, *usage)
}
