package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An effort asks for adaptive thinking at that effort; without one the
// request leaves both to the model's defaults.
func TestParamsEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		effort  string
		want    []string
		without []string
	}{
		{effort: "", without: []string{`"output_config"`, `"thinking"`}},
		{effort: "low", want: []string{`"output_config":{"effort":"low"}`, `"thinking":{"type":"adaptive"}`}},
		{effort: "high", want: []string{`"output_config":{"effort":"high"}`, `"thinking":{"type":"adaptive"}`}},
	}
	for _, tt := range tests {
		t.Run(tt.effort, func(t *testing.T) {
			t.Parallel()

			a := &Anthropic{model: "claude-test", effort: tt.effort}
			body, err := json.Marshal(a.params("system", "prompt"))
			require.NoError(t, err)
			for _, s := range tt.want {
				assert.Contains(t, string(body), s)
			}
			for _, s := range tt.without {
				assert.NotContains(t, string(body), s)
			}
		})
	}
}
