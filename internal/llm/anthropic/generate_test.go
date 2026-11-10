package anthropic

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/ionut-t/bark/v2/internal/llm/llmtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Generate reads every content block, not only the first: thinking blocks
// can come before the text, and the text can be split across blocks.
func TestGenerateContentBlocks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		content   string
		stop      string
		want      string
		wantErrIn []string
	}{
		{
			name:    "first block is not text",
			content: `{"type":"thinking","thinking":"hmm","signature":"sig"},{"type":"text","text":"answer"}`,
			stop:    "end_turn",
			want:    "answer",
		},
		{
			name:    "text split across blocks",
			content: `{"type":"text","text":"first "},{"type":"thinking","thinking":"hmm","signature":"sig"},{"type":"text","text":"second"}`,
			stop:    "end_turn",
			want:    "first second",
		},
		{
			name:      "only empty text",
			content:   `{"type":"text","text":""}`,
			stop:      "max_tokens",
			wantErrIn: []string{"text", "max_tokens"},
		},
		{
			name:      "no text blocks",
			content:   `{"type":"thinking","thinking":"hmm","signature":"sig"},{"type":"redacted_thinking","data":"x"}`,
			stop:      "max_tokens",
			wantErrIn: []string{"thinking", "redacted_thinking", "max_tokens"},
		},
		{
			name:      "no blocks",
			content:   ``,
			stop:      "refusal",
			wantErrIn: []string{"refusal"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := `{"id":"m","type":"message","role":"assistant","model":"claude-test",` +
				`"content":[` + tt.content + `],"stop_reason":"` + tt.stop + `",` +
				`"usage":{"input_tokens":10,"output_tokens":5}}`
			a := &Anthropic{model: "claude-test", client: anthropic.NewClient(
				option.WithAPIKey("test"),
				option.WithBaseURL("http://llm.test"),
				option.WithHTTPClient(llmtest.JSONClient(body)),
				option.WithMaxRetries(0),
			)}

			resp, err := a.Generate(t.Context(), "", "prompt")
			if tt.wantErrIn != nil {
				require.Error(t, err)
				for _, s := range tt.wantErrIn {
					assert.ErrorContains(t, err, s)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, resp.Content)
			require.NotNil(t, resp.Usage)
			assert.Equal(t, int64(15), resp.Usage.TotalTokens)
		})
	}
}
