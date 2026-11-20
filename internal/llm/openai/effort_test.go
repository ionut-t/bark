package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An effort is sent as the reasoning effort; without one the request
// leaves it to the model's default.
func TestParamsEffort(t *testing.T) {
	t.Parallel()

	for effort, want := range map[string]string{"": "", "low": "low", "medium": "medium", "high": "high"} {
		t.Run(effort, func(t *testing.T) {
			t.Parallel()

			o := &OpenAI{model: "gpt-test", effort: effort}
			body, err := json.Marshal(o.params("system", "prompt"))
			require.NoError(t, err)
			if want == "" {
				assert.NotContains(t, string(body), `"reasoning"`)
				return
			}
			assert.Contains(t, string(body), `"reasoning":{"effort":"`+want+`"}`)
		})
	}
}
