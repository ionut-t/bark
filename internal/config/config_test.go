package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
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
		Editor:             "nano",
		LLMProvider:        "openai",
		LLMModel:           "gpt-x",
		MaxDiffLines:       300,
		RelativeNumber:     true,
		ContextEnrichment:  true,
		StructuredFindings: true,
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
		{
			name: "structured findings",
			set:  func(c Config) error { return c.SetStructuredFindings(false) },
			want: func(d Data) Data { d.StructuredFindings = false; return d },
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

// Overrides apply to one run only and are never saved.
func TestNew_OverridesAreNotSaved(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), configFileName)
	data := Data{Editor: "nano", LLMProvider: "openai", LLMModel: "gpt-x", MaxDiffLines: 300}
	cfg := New(path, data)

	cfg.OverrideModel("other-model")
	require.NoError(t, cfg.OverrideProvider("anthropic"))
	cfg.OverrideMaxDiffLines(10)
	cfg.OverrideContextEnrichment(true)
	cfg.OverrideStructuredFindings(true)
	require.NoError(t, cfg.SetRelativeNumber(true))

	data.RelativeNumber = true
	assert.Equal(t, data, readSaved(t, path))
}

// Saving an unrelated setting keeps the overrides for the current run.
func TestNew_OverridesSurviveSetters(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), configFileName)
	cfg := New(path, Data{Editor: "nano", LLMProvider: "openai", LLMModel: "gpt-x", MaxDiffLines: 300})

	cfg.OverrideModel("other-model")
	require.NoError(t, cfg.OverrideProvider("anthropic"))
	cfg.OverrideMaxDiffLines(10)
	cfg.OverrideContextEnrichment(true)
	cfg.OverrideStructuredFindings(true)
	require.NoError(t, cfg.SetRelativeNumber(true))

	model, err := cfg.GetLLMModel()
	require.NoError(t, err)
	assert.Equal(t, "other-model", model)

	provider, err := cfg.GetLLMProvider()
	require.NoError(t, err)
	assert.Equal(t, "anthropic", provider)

	assert.Equal(t, uint32(10), cfg.GetMaxDiffLines())
	assert.True(t, cfg.GetContextEnrichment())
	assert.True(t, cfg.GetStructuredFindings())
	assert.True(t, cfg.GetRelativeNumber())
}

// Setting an overridden field saves the new value and uses it from then on:
// the most recent change wins.
func TestNew_SetterReplacesOverride(t *testing.T) {
	t.Parallel()

	data := Data{Editor: "nano", LLMProvider: "openai", LLMModel: "gpt-x", MaxDiffLines: 300}

	tests := []struct {
		name     string
		override func(Config) error
		set      func(Config) error
		get      func(Config) any
		want     any
		saved    func(Data) Data
	}{
		{
			name:     "model",
			override: func(c Config) error { c.OverrideModel("other-model"); return nil },
			set:      func(c Config) error { return c.SetLLMModel("claude-sonnet-5-5") },
			get:      func(c Config) any { m, _ := c.GetLLMModel(); return m },
			want:     "claude-sonnet-5-5",
			saved:    func(d Data) Data { d.LLMModel = "claude-sonnet-5-5"; return d },
		},
		{
			name:     "model set back to the saved value",
			override: func(c Config) error { c.OverrideModel("other-model"); return nil },
			set:      func(c Config) error { return c.SetLLMModel("gpt-x") },
			get:      func(c Config) any { m, _ := c.GetLLMModel(); return m },
			want:     "gpt-x",
			saved:    func(d Data) Data { return d },
		},
		{
			name:     "provider",
			override: func(c Config) error { return c.OverrideProvider("anthropic") },
			set:      func(c Config) error { return c.SetLLMProvider("ollama") },
			get:      func(c Config) any { p, _ := c.GetLLMProvider(); return p },
			want:     "ollama",
			saved:    func(d Data) Data { d.LLMProvider = "ollama"; return d },
		},
		{
			name:     "max diff lines",
			override: func(c Config) error { c.OverrideMaxDiffLines(10); return nil },
			set:      func(c Config) error { return c.SetMaxDiffLines(0) },
			get:      func(c Config) any { return c.GetMaxDiffLines() },
			want:     uint32(0),
			saved:    func(d Data) Data { d.MaxDiffLines = 0; return d },
		},
		{
			name:     "context enrichment",
			override: func(c Config) error { c.OverrideContextEnrichment(true); return nil },
			set:      func(c Config) error { return c.SetContextEnrichment(false) },
			get:      func(c Config) any { return c.GetContextEnrichment() },
			want:     false,
			saved:    func(d Data) Data { return d },
		},
		{
			name:     "structured findings",
			override: func(c Config) error { c.OverrideStructuredFindings(true); return nil },
			set:      func(c Config) error { return c.SetStructuredFindings(false) },
			get:      func(c Config) any { return c.GetStructuredFindings() },
			want:     false,
			saved:    func(d Data) Data { return d },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), configFileName)
			require.NoError(t, writeConfig(path, data))
			cfg := New(path, data)

			require.NoError(t, tt.override(cfg))
			require.NoError(t, tt.set(cfg))

			assert.Equal(t, tt.want, tt.get(cfg))
			assert.Equal(t, tt.saved(data), readSaved(t, path))
		})
	}
}

// The TUI saves relative_number from a tea.Cmd goroutine while Update reads it, so a
// Config must be safe for concurrent use. Run with -race.
func TestConfig_ConcurrentUse(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), configFileName)
	cfg := New(path, Data{Editor: "nano", LLMProvider: "openai", LLMModel: "gpt-x", MaxDiffLines: 300})

	const n = 50
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { assert.NoError(t, cfg.SetRelativeNumber(i%2 == 0)) })
		wg.Go(func() { _ = cfg.GetRelativeNumber() })
		wg.Go(func() { assert.NoError(t, cfg.SetLLMModel(fmt.Sprintf("model-%d", i))) })
		wg.Go(func() { _, _ = cfg.GetLLMModel() })
		wg.Go(func() { assert.NoError(t, cfg.SetMaxDiffLines(uint32(i))) })
		wg.Go(func() { cfg.OverrideMaxDiffLines(uint32(i)) })
		wg.Go(func() { _ = cfg.GetMaxDiffLines() })
	}
	wg.Wait()

	// The file holds the last save, which is the current saved state.
	saved := readSaved(t, path)
	model, err := cfg.GetLLMModel()
	require.NoError(t, err)
	assert.Equal(t, cfg.GetRelativeNumber(), saved.RelativeNumber)
	assert.Equal(t, model, saved.LLMModel)
}
