package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
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

			cfg := &config{data: Data{LLMProvider: tt.configured}}

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

	field, ok := reflect.TypeFor[Data]().FieldByName("LLMProvider")
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

func readSaved(t *testing.T, path string) Data {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	var saved Data
	require.NoError(t, toml.Unmarshal(content, &saved))
	return saved
}

// Each Config writes to its own path, so configs don't share state.
func TestNew_SettersWriteEveryValueToItsOwnPath(t *testing.T) {
	t.Parallel()

	data := Data{
		Editor:            "nano",
		LLMProvider:       "openai",
		LLMModel:          "gpt-x",
		MaxDiffLines:      300,
		RelativeNumber:    true,
		ContextEnrichment: true,
	}

	tests := []struct {
		name string
		set  func(Config) error
		want func(Data) Data
	}{
		{
			name: "editor",
			set:  func(c Config) error { return c.SetEditor("vi") },
			want: func(d Data) Data { d.Editor = "vi"; return d },
		},
		{
			name: "provider",
			set:  func(c Config) error { return c.SetLLMProvider("anthropic") },
			want: func(d Data) Data { d.LLMProvider = "anthropic"; return d },
		},
		{
			name: "model",
			set:  func(c Config) error { return c.SetLLMModel("claude-sonnet-5-5") },
			want: func(d Data) Data { d.LLMModel = "claude-sonnet-5-5"; return d },
		},
		{
			name: "max diff lines",
			set:  func(c Config) error { return c.SetMaxDiffLines(0) },
			want: func(d Data) Data { d.MaxDiffLines = 0; return d },
		},
		{
			name: "relative number",
			set:  func(c Config) error { return c.SetRelativeNumber(false) },
			want: func(d Data) Data { d.RelativeNumber = false; return d },
		},
		{
			name: "context enrichment",
			set:  func(c Config) error { return c.SetContextEnrichment(false) },
			want: func(d Data) Data { d.ContextEnrichment = false; return d },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), configFileName)
			cfg := New(path, data)
			assert.Equal(t, path, cfg.Path())

			require.NoError(t, tt.set(cfg))
			assert.Equal(t, tt.want(data), readSaved(t, path))
		})
	}
}

func TestNew_EmptyEditorFallsBackToEnv(t *testing.T) {
	t.Setenv("EDITOR", "nano")

	assert.Equal(t, "nano", New("", Data{}).GetEditor())
	assert.Equal(t, "vi", New("", Data{Editor: "vi"}).GetEditor())
}

func TestEnsureFile_CreatesDefaultsOnce(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), configFileName)

	created, err := EnsureFile(path)
	require.NoError(t, err)
	assert.True(t, created)

	saved := readSaved(t, path)
	assert.Equal(t, "gemini-2.5-pro", saved.LLMModel)
	assert.Empty(t, saved.LLMProvider)
	assert.NotEmpty(t, saved.Editor)
	assert.Equal(t, uint32(DEFAULT_MAX_DIFF_LINES), saved.MaxDiffLines)

	const existing = "llm_model = 'kept'\n"
	require.NoError(t, os.WriteFile(path, []byte(existing), 0o644))

	created, err = EnsureFile(path)
	require.NoError(t, err)
	assert.False(t, created)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, existing, string(content))
}
