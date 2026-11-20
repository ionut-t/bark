package genai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

// Gemini 3 and later take a thinking level; Gemini 2.5 only takes a token
// budget. Without an effort the request leaves thinking to the model.
func TestConfigEffort(t *testing.T) {
	t.Parallel()

	budget := func(n int32) *int32 { return &n }
	tests := []struct {
		name   string
		model  string
		effort string
		want   *genai.ThinkingConfig
	}{
		{name: "no effort", model: "gemini-3.6-flash"},
		{name: "level low", model: "gemini-3.6-flash", effort: "low", want: &genai.ThinkingConfig{ThinkingLevel: genai.ThinkingLevelLow}},
		{name: "level medium", model: "gemini-3.1-pro-preview", effort: "medium", want: &genai.ThinkingConfig{ThinkingLevel: genai.ThinkingLevelMedium}},
		{name: "level high", model: "models/gemini-3.6-flash", effort: "high", want: &genai.ThinkingConfig{ThinkingLevel: genai.ThinkingLevelHigh}},
		{name: "2.5 low", model: "gemini-2.5-pro", effort: "low", want: &genai.ThinkingConfig{ThinkingBudget: budget(1024)}},
		{name: "2.5 medium", model: "gemini-2.5-flash", effort: "medium", want: &genai.ThinkingConfig{ThinkingBudget: budget(8192)}},
		{name: "2.5 high", model: "Gemini-2.5-Flash-Lite", effort: "high", want: &genai.ThinkingConfig{ThinkingBudget: budget(24576)}},
		// Vertex AI also takes resource names.
		{name: "2.5 publisher path", model: "publishers/google/models/gemini-2.5-flash", effort: "low", want: &genai.ThinkingConfig{ThinkingBudget: budget(1024)}},
		{name: "2.5 full resource path", model: "projects/p/locations/us-central1/publishers/google/models/gemini-2.5-pro", effort: "medium", want: &genai.ThinkingConfig{ThinkingBudget: budget(8192)}},
		{name: "level on a resource path", model: "publishers/google/models/gemini-3.6-flash", effort: "low", want: &genai.ThinkingConfig{ThinkingLevel: genai.ThinkingLevelLow}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := &GenAI{model: tt.model, effort: tt.effort}
			cfg := g.config("system")
			require.NotNil(t, cfg)
			assert.Equal(t, "system", cfg.SystemInstruction.Parts[0].Text)
			assert.Equal(t, tt.want, cfg.ThinkingConfig)
		})
	}
}

// With neither a system prompt nor an effort there is no config at all.
func TestConfigEmpty(t *testing.T) {
	t.Parallel()

	assert.Nil(t, (&GenAI{model: "gemini-3.6-flash"}).config(""))
	cfg := (&GenAI{model: "gemini-3.6-flash", effort: "low"}).config("")
	require.NotNil(t, cfg)
	assert.Nil(t, cfg.SystemInstruction)
	assert.Equal(t, genai.ThinkingLevelLow, cfg.ThinkingConfig.ThinkingLevel)
}
