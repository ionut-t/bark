package config

import (
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
