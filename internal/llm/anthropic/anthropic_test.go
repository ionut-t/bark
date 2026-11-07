package anthropic

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/ionut-t/bark/v2/internal/llm/llmtest"
)

func TestStream_StreamErrorAndCloseErrorDoNotHang(t *testing.T) {
	a := &Anthropic{
		model: "claude-test",
		client: anthropic.NewClient(
			option.WithAPIKey("test"),
			option.WithBaseURL("http://llm.test"),
			option.WithHTTPClient(llmtest.StreamErrorClient()),
			option.WithMaxRetries(0),
		),
	}

	out, errc := a.Stream(t.Context(), "", "prompt")
	llmtest.AssertStreamCloses(t, out, errc)
}
