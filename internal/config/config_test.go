package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOverrideProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		configured  string
		override    string
		want        string
		wantErr     bool
		wantNoValue bool
	}{
		{
			name:       "empty override keeps configured provider",
			configured: "anthropic",
			override:   "",
			want:       "anthropic",
		},
		{
			name:       "valid override replaces configured provider",
			configured: "anthropic",
			override:   "openai",
			want:       "openai",
		},
		{
			name:       "override applies when nothing is configured",
			configured: "",
			override:   "ollama",
			want:       "ollama",
		},
		{
			name:       "invalid override returns error and keeps configured provider",
			configured: "anthropic",
			override:   "not-a-provider",
			want:       "anthropic",
			wantErr:    true,
		},
		{
			name:        "empty override with nothing configured leaves provider unset for auto-detection",
			configured:  "",
			override:    "",
			wantNoValue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &config{data: configData{LLMProvider: tt.configured}}

			err := cfg.OverrideProvider(tt.override)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			got, err := cfg.GetLLMProvider()
			if tt.wantNoValue {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOverrideProvider_AcceptsEverySupportedProvider(t *testing.T) {
	t.Parallel()

	for _, provider := range SupportedProviders() {
		t.Run(provider, func(t *testing.T) {
			t.Parallel()

			cfg := &config{}
			require.NoError(t, cfg.OverrideProvider(provider))

			got, err := cfg.GetLLMProvider()
			require.NoError(t, err)
			assert.Equal(t, provider, got)
		})
	}
}

func TestOverrideProvider_InvalidErrorListsSupportedProviders(t *testing.T) {
	t.Parallel()

	cfg := &config{}
	err := cfg.OverrideProvider("not-a-provider")
	require.Error(t, err)

	for _, provider := range SupportedProviders() {
		assert.Contains(t, err.Error(), provider)
	}
}

// The config file comment is a struct tag, so it can't be built from the
// list; this catches it going stale when a provider is added.
func TestConfigFileCommentListsSupportedProviders(t *testing.T) {
	t.Parallel()

	field, ok := reflect.TypeFor[configData]().FieldByName("LLMProvider")
	require.True(t, ok)
	comment := strings.ToLower(field.Tag.Get("comment"))

	for _, provider := range SupportedProviders() {
		assert.Contains(t, comment, provider)
	}
}

func TestSupportedProvidersReturnsCopy(t *testing.T) {
	t.Parallel()

	providers := SupportedProviders()
	providers[0] = "changed"

	assert.NotEqual(t, "changed", SupportedProviders()[0])
}
