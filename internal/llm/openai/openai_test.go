package openai

import (
	"testing"

	"github.com/ionut-t/bark/v2/internal/llm/llmtest"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStream_StreamErrorAndCloseErrorDoNotHang(t *testing.T) {
	o := &OpenAI{
		model: "gpt-test",
		client: openai.NewClient(
			option.WithAPIKey("test"),
			option.WithBaseURL("http://llm.test"),
			option.WithHTTPClient(llmtest.StreamErrorClient()),
			option.WithMaxRetries(0),
		),
	}

	out, errc := o.Stream(t.Context(), "", "prompt")
	llmtest.AssertStreamCloses(t, out, errc)
}

func TestNew(t *testing.T) {
	c := New("some-model", "fake-key", "high")
	require.NotNil(t, c)
	assert.Equal(t, "some-model", c.model)
	assert.Equal(t, "high", c.effort)
}
