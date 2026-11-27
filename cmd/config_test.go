package cmd

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type savedConfig struct {
	Editor             string `toml:"editor"`
	LLMProvider        string `toml:"llm_provider"`
	LLMModel           string `toml:"llm_model"`
	MaxDiffTokens      uint32 `toml:"max_diff_tokens"`
	MaxDiffLines       uint32 `toml:"max_diff_lines"`
	ContextEnrichment  bool   `toml:"context_enrichment"`
	StructuredFindings bool   `toml:"structured_findings"`
	CheckFindings      bool   `toml:"check_findings"`
	MinSeverity        string `toml:"min_severity"`
	FailOn             string `toml:"fail_on"`
	Effort             string `toml:"effort"`
	LLMTimeout         string `toml:"llm_timeout"`
}

func readSavedConfig(t *testing.T, path string) savedConfig {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var saved savedConfig
	require.NoError(t, toml.Unmarshal(data, &saved))
	return saved
}

// runConfigCmd runs `bark config <args>` against a config file in a temp dir
// that starts with the given contents, and returns what was saved, without
// the editor. Each call loads its own Config, so tests can run in parallel.
func runConfigCmd(t *testing.T, existing string, args ...string) savedConfig {
	t.Helper()

	// If no flag counts as set, the command opens the editor instead; make
	// that a no-op so the test fails on the saved values rather than hanging.
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("editor = 'true'\n"+existing), 0o644))

	cfg, err := loadConfig(path)
	require.NoError(t, err)

	cmd := configCmd(staticConfig(cfg))
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	require.NoError(t, cmd.Execute())

	saved := readSavedConfig(t, path)
	saved.Editor = ""
	return saved
}

func TestConfigCmd_SetsValues(t *testing.T) {
	t.Parallel()

	base := savedConfig{
		LLMProvider:        "anthropic",
		LLMModel:           "claude-sonnet-5-5",
		ContextEnrichment:  true,
		StructuredFindings: true,
		CheckFindings:      true,
	}
	withTokens, withLines := base, base
	withTokens.MaxDiffTokens = 8000
	withLines.MaxDiffLines = 500

	tests := []struct {
		name string
		args []string
		want savedConfig
	}{
		{
			name: "long flags",
			args: []string{"--provider", "anthropic", "--model", "claude-sonnet-5-5", "--max-diff-tokens", "8000", "--context-enrichment", "--structured-findings", "--check-findings"},
			want: withTokens,
		},
		{
			name: "short flags",
			args: []string{"-p", "anthropic", "-m", "claude-sonnet-5-5", "--max-diff-tokens", "8000", "--context-enrichment", "--structured-findings", "--check-findings"},
			want: withTokens,
		},
		{
			name: "deprecated flags still work",
			args: []string{"--llm_provider", "anthropic", "--llm_model", "claude-sonnet-5-5", "--max_diff_lines", "500", "--context_enrichment", "--structured-findings", "--check-findings"},
			want: withLines,
		},
		{
			name: "deprecated --max-diff-lines still works",
			args: []string{"-p", "anthropic", "-m", "claude-sonnet-5-5", "--max-diff-lines", "500", "--context-enrichment", "--structured-findings", "--check-findings"},
			want: withLines,
		},
		{
			name: "deprecated -d still works",
			args: []string{"-p", "anthropic", "-m", "claude-sonnet-5-5", "-d", "500", "--context-enrichment", "--structured-findings", "--check-findings"},
			want: withLines,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, runConfigCmd(t, "", tt.args...))
		})
	}
}

// Zero and false are real settings (0 disables the diff limit), so passing
// them must save them rather than be treated as "flag not given".
func TestConfigCmd_SetsZeroValues(t *testing.T) {
	t.Parallel()

	const existing = "max_diff_tokens = 9000\nmax_diff_lines = 300\ncontext_enrichment = true\nstructured_findings = true\ncheck_findings = true\n"
	base := savedConfig{LLMModel: config.DefaultData().LLMModel, MaxDiffTokens: 9000, MaxDiffLines: 300, ContextEnrichment: true, StructuredFindings: true, CheckFindings: true}
	with := func(change func(*savedConfig)) savedConfig {
		c := base
		change(&c)
		return c
	}

	tests := []struct {
		name string
		args []string
		want savedConfig
	}{
		{name: "--max-diff-tokens 0", args: []string{"--max-diff-tokens", "0"}, want: with(func(c *savedConfig) { c.MaxDiffTokens = 0 })},
		{name: "-d 0", args: []string{"-d", "0"}, want: with(func(c *savedConfig) { c.MaxDiffLines = 0 })},
		{name: "--max-diff-lines 0", args: []string{"--max-diff-lines", "0"}, want: with(func(c *savedConfig) { c.MaxDiffLines = 0 })},
		{name: "--max_diff_lines 0", args: []string{"--max_diff_lines", "0"}, want: with(func(c *savedConfig) { c.MaxDiffLines = 0 })},
		{name: "--context-enrichment=false", args: []string{"--context-enrichment=false"}, want: with(func(c *savedConfig) { c.ContextEnrichment = false })},
		{name: "--context_enrichment=false", args: []string{"--context_enrichment=false"}, want: with(func(c *savedConfig) { c.ContextEnrichment = false })},
		{name: "--structured-findings=false", args: []string{"--structured-findings=false"}, want: with(func(c *savedConfig) { c.StructuredFindings = false })},
		{name: "--check-findings=false", args: []string{"--check-findings=false"}, want: with(func(c *savedConfig) { c.CheckFindings = false })},
		{name: "--min-severity high", args: []string{"--min-severity", "high"}, want: with(func(c *savedConfig) { c.MinSeverity = "high" })},
		{name: "--fail-on critical", args: []string{"--fail-on", "critical"}, want: with(func(c *savedConfig) { c.FailOn = "critical" })},
		{name: "--effort low", args: []string{"--effort", "Low"}, want: with(func(c *savedConfig) { c.Effort = "low" })},
		{name: "--timeout 10m", args: []string{"--timeout", "10m"}, want: with(func(c *savedConfig) { c.LLMTimeout = "10m" })},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, runConfigCmd(t, existing, tt.args...))
		})
	}
}

// Setting one value must keep the others already in the file.
func TestConfigCmd_KeepsOtherValues(t *testing.T) {
	t.Parallel()

	const existing = "llm_provider = 'openai'\nllm_model = 'gpt-x'\nmax_diff_lines = 300\ncontext_enrichment = true\n"

	got := runConfigCmd(t, existing, "-e", "nano")
	assert.Equal(t, savedConfig{LLMProvider: "openai", LLMModel: "gpt-x", MaxDiffLines: 300, ContextEnrichment: true, CheckFindings: true}, got)
}

func TestConfigCmd_OldFlagNamesAreHiddenAndDeprecated(t *testing.T) {
	t.Parallel()

	renamed := map[string]string{
		config.LLMProviderKey:       "provider",
		config.LLMModelKey:          "model",
		config.MaxDiffLinesKey:      "max-diff-tokens",
		config.ContextEnrichmentKey: "context-enrichment",
	}

	flags := configCmd(staticConfig(config.New("", config.Data{}))).Flags()
	for oldName, newName := range renamed {
		old := flags.Lookup(oldName)
		require.NotNil(t, old, oldName)
		assert.True(t, old.Hidden, "%s should be hidden", oldName)
		assert.Contains(t, old.Deprecated, "--"+newName)
		assert.Empty(t, old.Shorthand, "%s should not keep a shorthand", oldName)

		require.NotNil(t, flags.Lookup(newName), newName)
	}
}

// --max-diff-lines is deprecated in favour of --max-diff-tokens: hidden from
// help, with a notice pointing at the new flag.
func TestMaxDiffLinesFlagIsDeprecated(t *testing.T) {
	t.Parallel()

	getCfg := staticConfig(config.New("", config.Data{}))
	for name, cmd := range map[string]*cobra.Command{
		"config": configCmd(getCfg),
		"review": reviewCmd(getCfg),
		"pr":     prCmd(getCfg),
	} {
		old := cmd.Flags().Lookup("max-diff-lines")
		require.NotNil(t, old, name)
		assert.True(t, old.Hidden, "%s: --max-diff-lines should be hidden", name)
		assert.Contains(t, old.Deprecated, "--max-diff-tokens", name)
		require.NotNil(t, cmd.Flags().Lookup("max-diff-tokens"), name)
	}
}

// A new config file doesn't mention the deprecated max_diff_lines.
func TestLoadConfig_NewFileLeavesOutMaxDiffLines(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	_, err := loadConfig(path)
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), config.MaxDiffTokensKey)
	assert.NotContains(t, string(content), config.MaxDiffLinesKey)
}

// Setting the editor saves it alongside the other values.
func TestConfigCmd_SetsEditor(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("editor = 'vi'\nllm_model = 'gpt-x'\n"), 0o644))

	cfg, err := loadConfig(path)
	require.NoError(t, err)

	cmd := configCmd(staticConfig(cfg))
	cmd.SetArgs([]string{"-e", "nano"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, savedConfig{Editor: "nano", LLMModel: "gpt-x", ContextEnrichment: true, CheckFindings: true}, readSavedConfig(t, path))
}

// Every field saved to the config file must be read back. Catches a field
// added to config.Data that loading doesn't pick up.
func TestReadConfigData_ReadsEveryField(t *testing.T) {
	t.Parallel()

	want := config.Data{
		Editor:             "nano",
		LLMProvider:        "anthropic",
		LLMModel:           "claude-sonnet-5-5",
		MaxDiffTokens:      9000,
		MaxDiffLines:       300,
		RelativeNumber:     true,
		ContextEnrichment:  false,
		StructuredFindings: true,
		CheckFindings:      false,
		MinSeverity:        "high",
		FailOn:             "critical",
		HouseRules:         false,
		HouseRulesFiles:    []string{"docs/STYLE.md"},
		Effort:             "low",
		LLMTimeout:         "10m",
		PreferredReviewers: []string{"Yoda", "Linus"},
	}

	// Loading starts from the defaults, so a field left at its default here
	// would pass even if it isn't read.
	fields := reflect.ValueOf(want)
	defaults := reflect.ValueOf(config.DefaultData())
	for i := range fields.NumField() {
		require.NotEqual(t, defaults.Field(i).Interface(), fields.Field(i).Interface(),
			"set %s to a value other than its default in this test", fields.Type().Field(i).Name)
	}

	out, err := toml.Marshal(want)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, out, 0o644))

	got, err := readConfigData(path)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// Keys missing from the file load as defaults, not Go zero values; keys
// present in the file win, even when they hold the zero value.
func TestReadConfigData_MissingKeysUseDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contents string
		want     config.Data
	}{
		{
			name:     "only editor",
			contents: "editor = 'nano'\n",
			want:     withDefaults(func(d *config.Data) { d.Editor = "nano" }),
		},
		{
			name:     "only provider and model",
			contents: "llm_provider = 'anthropic'\nllm_model = 'custom-model'\n",
			want: withDefaults(func(d *config.Data) {
				d.LLMProvider = "anthropic"
				d.LLMModel = "custom-model"
			}),
		},
		{
			name:     "empty file",
			contents: "",
			want:     withDefaults(func(*config.Data) {}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "config.toml")
			require.NoError(t, os.WriteFile(path, []byte(tt.contents), 0o644))

			got, err := readConfigData(path)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Saving one setting to a partial file must write defaults for the missing
// keys, not zeros.
func TestConfigCmd_PartialFileSavesDefaults(t *testing.T) {
	t.Parallel()

	want := withDefaults(func(d *config.Data) { d.LLMProvider = "anthropic" })

	got := runConfigCmd(t, "", "-p", "anthropic")
	assert.Equal(t, savedConfig{
		LLMProvider:       want.LLMProvider,
		LLMModel:          want.LLMModel,
		MaxDiffTokens:     want.MaxDiffTokens,
		MaxDiffLines:      want.MaxDiffLines,
		ContextEnrichment: want.ContextEnrichment,
		CheckFindings:     want.CheckFindings,
	}, got)
}

// withDefaults returns the default config data with change applied, so a
// test lists only the fields that differ from the defaults.
func withDefaults(change func(*config.Data)) config.Data {
	d := config.DefaultData()
	change(&d)
	return d
}

func TestLoadConfig_ReadsFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	const contents = `editor = 'nano'
llm_provider = 'anthropic'
llm_model = 'claude-sonnet-5-5'
max_diff_tokens = 9000
max_diff_lines = 300
relative_number = true
context_enrichment = true
structured_findings = true
`
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))

	cfg, err := loadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, path, cfg.Path())
	assert.Equal(t, "nano", cfg.GetEditor())
	provider, err := cfg.GetLLMProvider()
	require.NoError(t, err)
	assert.Equal(t, "anthropic", provider)
	model, err := cfg.GetLLMModel()
	require.NoError(t, err)
	assert.Equal(t, "claude-sonnet-5-5", model)
	assert.Equal(t, uint32(9000), cfg.GetMaxDiffTokens())
	assert.Equal(t, uint32(300), cfg.GetMaxDiffLines())
	assert.True(t, cfg.GetRelativeNumber())
	assert.True(t, cfg.GetStructuredFindings())
	assert.True(t, cfg.GetContextEnrichment())
}

func TestLoadConfig_CreatesMissingFileWithDefaults(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")

	cfg, err := loadConfig(path)
	require.NoError(t, err)

	model, err := cfg.GetLLMModel()
	require.NoError(t, err)
	assert.Equal(t, "gemini-2.5-pro", model)
	_, err = cfg.GetLLMProvider()
	assert.Error(t, err, "provider should be unset so it is auto-detected")

	saved := readSavedConfig(t, path)
	assert.Equal(t, "gemini-2.5-pro", saved.LLMModel)
	assert.NotEmpty(t, saved.Editor)
}

func TestLoadConfig_InvalidFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("not = = toml"), 0o644))

	_, err := loadConfig(path)
	assert.Error(t, err)
}

func TestLoadConfigFrom_MissingFileIsAnError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing.toml")

	_, err := loadConfigFrom(path)

	require.ErrorIs(t, err, os.ErrNotExist)
	assert.NoFileExists(t, path, "a mistyped --config path must not be created")
}

func TestLoadConfigFrom_ReadsGivenFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "custom.toml")
	require.NoError(t, os.WriteFile(path, []byte("llm_provider = 'anthropic'\nllm_model = 'custom-model'\n"), 0o644))

	cfg, err := loadConfigFrom(path)
	require.NoError(t, err)

	assert.Equal(t, path, cfg.Path())
	provider, err := cfg.GetLLMProvider()
	require.NoError(t, err)
	assert.Equal(t, "anthropic", provider)

	model, err := cfg.GetLLMModel()
	require.NoError(t, err)
	assert.Equal(t, "custom-model", model)
}

// The config file is TOML whatever its name, so --config works with any
// extension.
func TestReadConfigData_AnyFileName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"config.toml", "bark.conf", "barkrc"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), name)
			require.NoError(t, os.WriteFile(path, []byte("llm_model = 'custom-model'\n"), 0o644))

			got, err := readConfigData(path)
			require.NoError(t, err)
			assert.Equal(t, withDefaults(func(d *config.Data) { d.LLMModel = "custom-model" }), got)
		})
	}
}

// A value of the wrong type is an error, not silently converted.
func TestReadConfigData_InvalidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		contents string
	}{
		{name: "negative max_diff_lines", contents: "max_diff_lines = -1\n"},
		{name: "quoted number", contents: "max_diff_lines = '300'\n"},
		{name: "quoted bool", contents: "relative_number = 'true'\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "config.toml")
			require.NoError(t, os.WriteFile(path, []byte(tt.contents), 0o644))

			got, err := readConfigData(path)
			require.Error(t, err, "loaded %+v", got)
			assert.Contains(t, err.Error(), path+":1:")
		})
	}
}

func TestReadConfigData_InvalidLLMTimeout(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("llm_timeout = '300'\n"), 0o644))

	_, err := readConfigData(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.Contains(t, err.Error(), "llm_timeout")
}

// An invalid --timeout is rejected without saving anything.
func TestConfigCmd_InvalidTimeout(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("llm_timeout = '10m'\n"), 0o644))
	cfg, err := loadConfig(path)
	require.NoError(t, err)

	cmd := configCmd(staticConfig(cfg))
	cmd.SetArgs([]string{"--timeout", "0s"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	require.ErrorContains(t, cmd.Execute(), "must be more than 0")
	assert.Equal(t, "10m", readSavedConfig(t, path).LLMTimeout)
}

func TestReadConfigData_InvalidEffort(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("effort = 'ludicrous'\n"), 0o644))

	_, err := readConfigData(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.Contains(t, err.Error(), "ludicrous")
}

func TestReadConfigData_InvalidMinSeverity(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("min_severity = 'nuclear'\n"), 0o644))

	_, err := readConfigData(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.Contains(t, err.Error(), "nuclear")
}

func TestReadConfigData_InvalidFailOn(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("fail_on = 'nuclear'\n"), 0o644))

	_, err := readConfigData(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.Contains(t, err.Error(), "fail_on")
	assert.Contains(t, err.Error(), "nuclear")
}

func TestReadConfigData_InvalidHouseRulesFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("house_rules_files = ['AGENTS.md', '../secrets.md']\n"), 0o644))

	_, err := readConfigData(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.Contains(t, err.Error(), "house_rules_files")
	assert.Contains(t, err.Error(), "../secrets.md")
}

func TestReadConfigData_BlankPreferredReviewer(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("preferred_reviewers = ['Yoda', '']\n"), 0o644))

	_, err := readConfigData(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.Contains(t, err.Error(), "preferred_reviewers entry 2")
}

// A syntax error says which file and line to fix.
func TestReadConfigData_ErrorNamesFileAndLine(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("editor = 'nano'\nnot = = toml\n"), 0o644))

	_, err := readConfigData(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), path+":2:7:")
	assert.Contains(t, err.Error(), "unexpected character")
}
