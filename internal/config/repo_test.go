package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRepo(t *testing.T) {
	t.Parallel()

	content := `
llm_provider = "anthropic"
llm_model = "claude-sonnet-5-5"
max_diff_lines = 0
context_enrichment = true
structured_findings = true
check_findings = false
min_severity = "High"
`
	got, err := ParseRepo("/repo/.bark/config.toml", []byte(content))
	require.NoError(t, err)
	assert.Equal(t, RepoData{
		LLMProvider:        new("anthropic"),
		LLMModel:           new("claude-sonnet-5-5"),
		MaxDiffLines:       new(uint32(0)),
		ContextEnrichment:  new(true),
		StructuredFindings: new(true),
		CheckFindings:      new(false),
		MinSeverity:        new("high"),
	}, got)

	empty, err := ParseRepo("/repo/.bark/config.toml", nil)
	require.NoError(t, err)
	assert.Equal(t, RepoData{}, empty)
}

func TestParseRepoErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			// The editor is run as a command: a cloned repository mustn't
			// be able to set it.
			name:    "editor is a personal setting",
			content: "editor = 'sh -c evil'\n",
			want:    []string{"config.toml", "editor"},
		},
		{
			name:    "relative_number is a personal setting",
			content: "relative_number = true\n",
			want:    []string{"relative_number"},
		},
		{
			name:    "misspelt key",
			content: "llm_modle = 'x'\n",
			want:    []string{"llm_modle"},
		},
		{
			name:    "unknown provider",
			content: "llm_provider = 'skynet'\n",
			want:    []string{"config.toml", "skynet", "gemini"},
		},
		{
			name:    "unknown severity",
			content: "min_severity = 'nuclear'\n",
			want:    []string{"config.toml", "nuclear", "critical, high, medium, low"},
		},
		{
			name:    "wrong type",
			content: "max_diff_lines = 'many'\n",
			want:    []string{"config.toml:1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseRepo("/repo/.bark/config.toml", []byte(tt.content))
			require.Error(t, err)
			for _, w := range tt.want {
				assert.Contains(t, err.Error(), w)
			}
		})
	}
}

func TestRepoFilePath(t *testing.T) {
	t.Parallel()
	assert.Equal(t, filepath.Join("/repo", ".bark", "config.toml"), RepoFilePath("/repo"))
}

func TestApplyRepo_RepoBeatsSavedAndRunOverridesBeatRepo(t *testing.T) {
	t.Parallel()

	saved := Data{
		LLMProvider:        "gemini",
		LLMModel:           "gemini-2.5-pro",
		MaxDiffLines:       500,
		ContextEnrichment:  false,
		StructuredFindings: false,
		CheckFindings:      true,
		MinSeverity:        "low",
	}
	repo := RepoData{
		LLMProvider:        new("anthropic"),
		LLMModel:           new("claude-sonnet-5-5"),
		MaxDiffLines:       new(uint32(0)),
		ContextEnrichment:  new(true),
		StructuredFindings: new(true),
		CheckFindings:      new(false),
		MinSeverity:        new("high"),
	}

	cfg := New("", saved)
	cfg.ApplyRepo("/repo/.bark/config.toml", repo)

	provider, _ := cfg.GetLLMProvider()
	model, _ := cfg.GetLLMModel()
	assert.Equal(t, "anthropic", provider)
	assert.Equal(t, "claude-sonnet-5-5", model)
	assert.Equal(t, uint32(0), cfg.GetMaxDiffLines(), "a repo can set a zero value")
	assert.True(t, cfg.GetContextEnrichment())
	assert.True(t, cfg.GetStructuredFindings())
	assert.False(t, cfg.GetCheckFindings())
	assert.Equal(t, "high", cfg.GetMinSeverity())

	require.NoError(t, cfg.OverrideProvider("openai"))
	cfg.OverrideModel("gpt-x")
	cfg.OverrideMaxDiffLines(10)
	cfg.OverrideContextEnrichment(false)
	cfg.OverrideStructuredFindings(false)
	cfg.OverrideCheckFindings(true)
	require.NoError(t, cfg.OverrideMinSeverity("medium"))

	provider, _ = cfg.GetLLMProvider()
	model, _ = cfg.GetLLMModel()
	assert.Equal(t, "openai", provider)
	assert.Equal(t, "gpt-x", model)
	assert.Equal(t, uint32(10), cfg.GetMaxDiffLines())
	assert.False(t, cfg.GetContextEnrichment())
	assert.False(t, cfg.GetStructuredFindings())
	assert.True(t, cfg.GetCheckFindings())
	assert.Equal(t, "medium", cfg.GetMinSeverity())
}

func TestApplyRepo_UnsetKeysUseSavedValues(t *testing.T) {
	t.Parallel()

	cfg := New("", Data{LLMProvider: "gemini", LLMModel: "gemini-2.5-pro", MaxDiffLines: 500, CheckFindings: true})
	cfg.ApplyRepo("/repo/.bark/config.toml", RepoData{StructuredFindings: new(true)})

	provider, _ := cfg.GetLLMProvider()
	model, _ := cfg.GetLLMModel()
	assert.Equal(t, "gemini", provider)
	assert.Equal(t, "gemini-2.5-pro", model)
	assert.Equal(t, uint32(500), cfg.GetMaxDiffLines())
	assert.True(t, cfg.GetCheckFindings())
	assert.True(t, cfg.GetStructuredFindings())
	assert.Empty(t, cfg.GetMinSeverity())
}

func TestApplyRepo_SettersSaveTheUserFileOnly(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), configFileName)
	saved := Data{Editor: "vi", LLMModel: "gemini-2.5-pro", CheckFindings: true}
	cfg := New(path, saved)
	cfg.ApplyRepo("/repo/.bark/config.toml", RepoData{LLMModel: new("claude-sonnet-5-5")})

	require.NoError(t, cfg.SetLLMModel("gemini-3.6-flash"))

	saved.LLMModel = "gemini-3.6-flash"
	assert.Equal(t, saved, readSaved(t, path), "the repo's values are never written to the user's file")
	model, _ := cfg.GetLLMModel()
	assert.Equal(t, "claude-sonnet-5-5", model, "the repo still wins in its own directory")
}

func TestRepoLLMWarning(t *testing.T) {
	t.Parallel()

	const path = "/repo/.bark/config.toml"
	saved := Data{LLMProvider: "gemini", LLMModel: "gemini-2.5-pro"}

	tests := []struct {
		name     string
		saved    Data
		repo     RepoData
		provider string
		model    string
		want     []string
		notWant  []string
	}{
		{
			name:  "no repo values",
			saved: saved,
			repo:  RepoData{StructuredFindings: new(true)},
		},
		{
			name:  "repo values equal to the user's",
			saved: saved,
			repo:  RepoData{LLMProvider: new("gemini"), LLMModel: new("gemini-2.5-pro")},
		},
		{
			name:  "provider and model",
			saved: saved,
			repo:  RepoData{LLMProvider: new("anthropic"), LLMModel: new("claude-opus-5-5")},
			want:  []string{path, `llm_provider = "anthropic"`, `llm_model = "claude-opus-5-5"`, `"gemini"`, `"gemini-2.5-pro"`, "--provider", "--model"},
		},
		{
			name:    "model only",
			saved:   saved,
			repo:    RepoData{LLMModel: new("gemini-3.1-pro-preview")},
			want:    []string{`llm_model = "gemini-3.1-pro-preview"`, `"gemini-2.5-pro"`},
			notWant: []string{"llm_provider"},
		},
		{
			name:  "provider when the user's is auto-detected",
			saved: Data{LLMModel: "gemini-2.5-pro"},
			repo:  RepoData{LLMProvider: new("openai")},
			want:  []string{`llm_provider = "openai"`, "auto-detected"},
		},
		{
			name:     "a flag replaces the repo's value",
			saved:    saved,
			repo:     RepoData{LLMProvider: new("anthropic"), LLMModel: new("claude-opus-5-5")},
			model:    "gemini-3.6-flash",
			want:     []string{`llm_provider = "anthropic"`},
			notWant:  []string{"llm_model"},
			provider: "",
		},
		{
			name:     "flags replace both",
			saved:    saved,
			repo:     RepoData{LLMProvider: new("anthropic"), LLMModel: new("claude-opus-5-5")},
			provider: "gemini",
			model:    "gemini-3.6-flash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := New("", tt.saved)
			cfg.ApplyRepo(path, tt.repo)
			require.NoError(t, cfg.OverrideProvider(tt.provider))
			cfg.OverrideModel(tt.model)

			got := cfg.RepoLLMWarning()
			if len(tt.want) == 0 {
				assert.Empty(t, got)
				return
			}
			for _, w := range tt.want {
				assert.Contains(t, got, w)
			}
			for _, w := range tt.notWant {
				assert.NotContains(t, got, w)
			}
		})
	}
}

func TestMinSeverity(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), configFileName)
	cfg := New(path, Data{Editor: "vi"})
	assert.Empty(t, cfg.GetMinSeverity(), "no threshold by default")

	require.NoError(t, cfg.SetMinSeverity("Medium"))
	assert.Equal(t, "medium", cfg.GetMinSeverity())
	assert.Equal(t, "medium", readSaved(t, path).MinSeverity)

	assert.Error(t, cfg.SetMinSeverity("nuclear"))
	assert.Error(t, cfg.OverrideMinSeverity("nuclear"))
	assert.Equal(t, "medium", cfg.GetMinSeverity())

	require.NoError(t, cfg.SetMinSeverity(""))
	assert.Empty(t, cfg.GetMinSeverity(), "an empty value turns the threshold off")
}
