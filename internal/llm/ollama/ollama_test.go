package ollama

import (
	"testing"

	"github.com/ionut-t/bark/v2/internal/llm/llmtest"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func TestStream_StreamErrorAndCloseErrorDoNotHang(t *testing.T) {
	o := &Ollama{
		model: "llama-test",
		client: openai.NewClient(
			option.WithAPIKey("ollama"),
			option.WithBaseURL("http://llm.test/v1"),
			option.WithHTTPClient(llmtest.StreamErrorClient()),
			option.WithMaxRetries(0),
		),
	}

	out, errc := o.Stream(t.Context(), "", "prompt")
	llmtest.AssertStreamCloses(t, out, errc)
}
