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
