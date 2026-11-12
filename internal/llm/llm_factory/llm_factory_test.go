package llm_factory

import (
	"context"
	"testing"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The configured provider must win over auto-detection when the --provider
// flag is not passed, even if credentials for other providers are present.
func TestNewUsesConfiguredProviderWithoutFlag(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "fake")
	t.Setenv("ANTHROPIC_API_KEY", "fake")

	cfg := config.New("", config.Data{LLMProvider: "anthropic", LLMModel: "claude-sonnet-5-5"})
	require.NoError(t, cfg.OverrideProvider("")) // what the commands do when -P is not passed

	_, provider, err := New(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, "anthropic", provider)
}

// Every provider the config accepts must be handled by the factory.
func TestValidateProvider_HandlesEverySupportedProvider(t *testing.T) {
	creds := &providerCredentials{
		vertexAIProjectID: "project",
		vertexAILocation:  "location",
		hasGemini:         true,
		hasVertexAI:       true,
		hasOpenAI:         true,
		hasOllama:         true,
		hasAnthropic:      true,
	}

	for _, provider := range config.SupportedProviders() {
		t.Run(provider, func(t *testing.T) {
			assert.NoError(t, creds.validateProvider(provider))
		})
	}
}

func TestValidateProvider_InvalidErrorListsSupportedProviders(t *testing.T) {
	err := (&providerCredentials{}).validateProvider("not-a-provider")
	require.ErrorIs(t, err, ErrInvalidProvider)

	for _, provider := range config.SupportedProviders() {
		assert.Contains(t, err.Error(), provider)
	}
}

// --model naming another provider's model must switch to that provider
// instead of sending it to the configured one.
func TestNewInfersProviderFromOverriddenModel(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "fake")
	t.Setenv("ANTHROPIC_API_KEY", "fake")

	cfg := config.New("", config.Data{LLMProvider: "gemini", LLMModel: "gemini-2.5-pro"})
	require.NoError(t, cfg.OverrideProvider(""))
	cfg.OverrideModel("claude-sonnet-5-5")

	_, provider, err := New(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, "anthropic", provider)
}

func TestResolveProvider(t *testing.T) {
	all := &providerCredentials{hasGemini: true, hasVertexAI: true, hasOpenAI: true, hasAnthropic: true, hasOllama: true}
	vertexOnly := &providerCredentials{hasVertexAI: true}
	geminiOnly := &providerCredentials{hasGemini: true}

	tests := []struct {
		name     string
		provider string
		pinned   bool
		model    string
		creds    *providerCredentials
		want     string
		wantErr  error
	}{
		// A pinned provider always wins, even over an obvious model name.
		{name: "pinned wins", provider: "openai", pinned: true, model: "claude-sonnet-5-5", creds: all, want: "openai"},
		{name: "pinned is normalised", provider: " Anthropic ", pinned: true, model: "x", creds: all, want: "anthropic"},
		{name: "pinned ollama with a gpt- name", provider: "ollama", pinned: true, model: "gpt-oss:20b", creds: all, want: "ollama"},

		// An unpinned provider gives way to a model that names another.
		{name: "claude", provider: "gemini", model: "claude-sonnet-5-5", creds: all, want: "anthropic"},
		{name: "gpt", provider: "gemini", model: "gpt-5", creds: all, want: "openai"},
		{name: "o-series", provider: "gemini", model: "o3-mini", creds: all, want: "openai"},
		{name: "fine-tune", provider: "gemini", model: "ft:gpt-4o-mini:org::abc", creds: all, want: "openai"},
		{name: "case-insensitive", provider: "gemini", model: "Claude-Opus-5-5", creds: all, want: "anthropic"},
		{name: "no provider at all", model: "claude-sonnet-5-5", creds: all, want: "anthropic"},

		// Gemini models run on both Gemini and Vertex AI.
		{name: "gemini keeps configured vertexai", provider: "vertexai", model: "gemini-3-pro", creds: all, want: "vertexai"},
		{name: "gemini keeps configured gemini", provider: "gemini", model: "gemini-3-pro", creds: all, want: "gemini"},
		{name: "gemini with vertex creds only", model: "gemini-3-pro", creds: vertexOnly, want: "vertexai"},
		{name: "gemini with both creds prefers gemini", provider: "openai", model: "gemini-3-pro", creds: all, want: "gemini"},
		{name: "models/ prefix", provider: "openai", model: "models/gemini-3-pro", creds: all, want: "gemini"},

		// Names Bark can't place fall back to today's behaviour; Ollama is
		// never inferred.
		{name: "unknown keeps configured", provider: "ollama", model: "llama3", creds: all, want: "ollama"},
		{name: "ollama tag isn't openai", provider: "ollama", model: "gpt-oss:20b", creds: all, want: "ollama"},
		{name: "unknown without provider detects", model: "llama3", creds: geminiOnly, want: "gemini"},
		{name: "unknown with nothing configured", model: "llama3", creds: &providerCredentials{}, wantErr: ErrNoProviderConfigured},

		// An inferred provider without credentials is still the answer, so
		// the error names the key the user needs.
		{name: "inferred without creds", provider: "gemini", model: "claude-sonnet-5-5", creds: geminiOnly, want: "anthropic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveProvider(tt.provider, tt.pinned, tt.model, tt.creds)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
