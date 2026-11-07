package llm_factory

import (
	"context"
	"testing"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The configured provider must win over auto-detection when the --provider
// flag is not passed, even if credentials for other providers are present.
func TestNewUsesConfiguredProviderWithoutFlag(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "fake")
	t.Setenv("ANTHROPIC_API_KEY", "fake")
	viper.Set(config.LLMProviderKey, "anthropic")
	viper.Set(config.LLMModelKey, "claude-sonnet-5-5")
	t.Cleanup(viper.Reset)

	cfg := config.New()
	require.NoError(t, cfg.OverrideProvider("")) // what the commands do when -P is not passed

	_, provider, err := New(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, "anthropic", provider)
}
