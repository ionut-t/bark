package anthropic

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/ionut-t/bark/v2/internal/llm/llmtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestNew(t *testing.T) {
	c := New("some-model", "fake-key", "high")
	require.NotNil(t, c)
	assert.Equal(t, "some-model", c.model)
	assert.Equal(t, "high", c.effort)
}
