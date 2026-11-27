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
max_diff_tokens = 9000
max_diff_lines = 0
context_enrichment = true
structured_findings = true
check_findings = false
min_severity = "High"
fail_on = "Critical"
`
	got, err := ParseRepo("/repo/.bark/config.toml", []byte(content))
	require.NoError(t, err)
	assert.Equal(t, RepoData{
		LLMProvider:        new("anthropic"),
		LLMModel:           new("claude-sonnet-5-5"),
		MaxDiffTokens:      new(uint32(9000)),
		MaxDiffLines:       new(uint32(0)),
		ContextEnrichment:  new(true),
		StructuredFindings: new(true),
		CheckFindings:      new(false),
		MinSeverity:        new("high"),
		FailOn:             new("critical"),
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
			name:    "unknown fail_on severity",
			content: "fail_on = 'nuclear'\n",
			want:    []string{"config.toml", "fail_on", "nuclear", "critical, high, medium, low"},
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
		MaxDiffTokens:      20000,
		MaxDiffLines:       500,
		ContextEnrichment:  false,
		StructuredFindings: false,
		CheckFindings:      true,
		MinSeverity:        "low",
		FailOn:             "low",
	}
	repo := RepoData{
		LLMProvider:        new("anthropic"),
		LLMModel:           new("claude-sonnet-5-5"),
		MaxDiffTokens:      new(uint32(0)),
		MaxDiffLines:       new(uint32(0)),
		ContextEnrichment:  new(true),
		StructuredFindings: new(true),
		CheckFindings:      new(false),
		MinSeverity:        new("high"),
		FailOn:             new("high"),
	}

	cfg := New("", saved)
	cfg.ApplyRepo("/repo/.bark/config.toml", repo)

	provider, _ := cfg.GetLLMProvider()
	model, _ := cfg.GetLLMModel()
	assert.Equal(t, "anthropic", provider)
	assert.Equal(t, "claude-sonnet-5-5", model)
	assert.Equal(t, uint32(0), cfg.GetMaxDiffTokens(), "a repo can set a zero value")
	assert.Equal(t, uint32(0), cfg.GetMaxDiffLines(), "a repo can set a zero value")
	assert.True(t, cfg.GetContextEnrichment())
	assert.True(t, cfg.GetStructuredFindings())
	assert.False(t, cfg.GetCheckFindings())
	assert.Equal(t, "high", cfg.GetMinSeverity())
	assert.Equal(t, "high", cfg.GetFailOn())

	require.NoError(t, cfg.OverrideProvider("openai"))
	cfg.OverrideModel("gpt-x")
	cfg.OverrideMaxDiffTokens(100)
	cfg.OverrideMaxDiffLines(10)
	cfg.OverrideContextEnrichment(false)
	cfg.OverrideStructuredFindings(false)
	cfg.OverrideCheckFindings(true)
	require.NoError(t, cfg.OverrideMinSeverity("medium"))
	require.NoError(t, cfg.OverrideFailOn(""))

	provider, _ = cfg.GetLLMProvider()
	model, _ = cfg.GetLLMModel()
	assert.Equal(t, "openai", provider)
	assert.Equal(t, "gpt-x", model)
	assert.Equal(t, uint32(100), cfg.GetMaxDiffTokens())
	assert.Equal(t, uint32(10), cfg.GetMaxDiffLines())
	assert.False(t, cfg.GetContextEnrichment())
	assert.False(t, cfg.GetStructuredFindings())
	assert.True(t, cfg.GetCheckFindings())
	assert.Equal(t, "medium", cfg.GetMinSeverity())
	assert.Empty(t, cfg.GetFailOn(), "an empty run override turns off the repository's fail_on")
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
	assert.Empty(t, cfg.GetFailOn())
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

// A provider pins the model only when it is set at the same layer as the
// model or a higher one (run > repo > user). A model chosen at a higher
// layer than the provider leaves the provider free to follow the model.
func TestLLMProviderChoice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		saved        Data
		repo         *RepoData
		overProvider string
		overModel    string
		wantProvider string
		wantPinned   bool
	}{
		{
			name:         "provider and model saved together",
			saved:        Data{LLMProvider: "ollama", LLMModel: "gpt-oss:20b"},
			wantProvider: "ollama",
			wantPinned:   true,
		},
		{
			name:         "no provider anywhere",
			saved:        Data{LLMModel: "claude-sonnet-5-5"},
			wantProvider: "",
			wantPinned:   false,
		},
		{
			name:         "--model over a saved provider",
			saved:        Data{LLMProvider: "gemini", LLMModel: "gemini-2.5-pro"},
			overModel:    "claude-sonnet-5-5",
			wantProvider: "gemini",
			wantPinned:   false,
		},
		{
			name:         "--provider and --model together",
			saved:        Data{LLMProvider: "gemini", LLMModel: "gemini-2.5-pro"},
			overProvider: "openai",
			overModel:    "claude-sonnet-5-5",
			wantProvider: "openai",
			wantPinned:   true,
		},
		{
			name:         "--provider alone",
			saved:        Data{LLMProvider: "gemini", LLMModel: "gemini-2.5-pro"},
			overProvider: "vertexai",
			wantProvider: "vertexai",
			wantPinned:   true,
		},
		{
			name:         "repo model over a saved provider",
			saved:        Data{LLMProvider: "gemini", LLMModel: "gemini-2.5-pro"},
			repo:         &RepoData{LLMModel: new("claude-sonnet-5-5")},
			wantProvider: "gemini",
			wantPinned:   false,
		},
		{
			name:         "repo sets both",
			saved:        Data{LLMProvider: "gemini", LLMModel: "gemini-2.5-pro"},
			repo:         &RepoData{LLMProvider: new("ollama"), LLMModel: new("llama3")},
			wantProvider: "ollama",
			wantPinned:   true,
		},
		{
			name:         "--model over a repo provider",
			saved:        Data{LLMModel: "gemini-2.5-pro"},
			repo:         &RepoData{LLMProvider: new("vertexai")},
			overModel:    "claude-sonnet-5-5",
			wantProvider: "vertexai",
			wantPinned:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := New("", tt.saved)
			if tt.repo != nil {
				cfg.ApplyRepo("/repo/.bark/config.toml", *tt.repo)
			}
			require.NoError(t, cfg.OverrideProvider(tt.overProvider))
			cfg.OverrideModel(tt.overModel)

			provider, pinned := cfg.LLMProviderChoice()
			assert.Equal(t, tt.wantProvider, provider)
			assert.Equal(t, tt.wantPinned, pinned)
		})
	}
}

func TestFailOn(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), configFileName)
	cfg := New(path, Data{Editor: "vi"})
	assert.Empty(t, cfg.GetFailOn(), "no threshold by default")

	require.NoError(t, cfg.SetFailOn("High"))
	assert.Equal(t, "high", cfg.GetFailOn())
	assert.Equal(t, "high", readSaved(t, path).FailOn)

	err := cfg.SetFailOn("nuclear")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fail_on", "the error names the setting")
	assert.Error(t, cfg.OverrideFailOn("nuclear"))
	assert.Equal(t, "high", cfg.GetFailOn())

	require.NoError(t, cfg.OverrideFailOn("critical"))
	assert.Equal(t, "critical", cfg.GetFailOn())
	assert.Equal(t, "high", readSaved(t, path).FailOn, "an override isn't saved")

	require.NoError(t, cfg.SetFailOn(""))
	assert.Empty(t, cfg.GetFailOn(), "an empty value turns the threshold off")
}

func TestHouseRules(t *testing.T) {
	t.Parallel()

	assert.True(t, DefaultData().HouseRules, "on by default")

	path := filepath.Join(t.TempDir(), configFileName)
	cfg := New(path, Data{Editor: "vi", HouseRules: true, HouseRulesFiles: []string{"AGENTS.md", "docs/STYLE.md"}})
	assert.True(t, cfg.GetHouseRules())
	assert.Equal(t, []string{"AGENTS.md", "docs/STYLE.md"}, cfg.GetHouseRulesFiles())

	cfg.ApplyRepo("/repo/.bark/config.toml", RepoData{HouseRules: new(false), HouseRulesFiles: &[]string{"RULES.md"}})
	assert.False(t, cfg.GetHouseRules(), "the repo beats the user")
	assert.Equal(t, []string{"RULES.md"}, cfg.GetHouseRulesFiles(), "the repo beats the user")

	cfg.OverrideHouseRules(true)
	assert.True(t, cfg.GetHouseRules(), "the run beats the repo")

	require.NoError(t, cfg.SetHouseRules(false))
	assert.False(t, readSaved(t, path).HouseRules)

	// An empty list in the repo means the defaults, even over a user's list.
	cfg.ApplyRepo("/repo/.bark/config.toml", RepoData{HouseRulesFiles: &[]string{}})
	assert.Empty(t, cfg.GetHouseRulesFiles())
}

func TestParseRepo_HouseRules(t *testing.T) {
	t.Parallel()

	got, err := ParseRepo("/repo/.bark/config.toml", []byte("house_rules = false\nhouse_rules_files = ['./AGENTS.md', 'docs/STYLE.md']\n"))
	require.NoError(t, err)
	assert.Equal(t, RepoData{HouseRules: new(false), HouseRulesFiles: &[]string{"./AGENTS.md", "docs/STYLE.md"}}, got)

	empty, err := ParseRepo("/repo/.bark/config.toml", []byte("house_rules_files = []\n"))
	require.NoError(t, err)
	require.NotNil(t, empty.HouseRulesFiles, "an empty list is set, not missing")
	assert.Empty(t, *empty.HouseRulesFiles)

	for _, bad := range []string{"/etc/passwd", "../x.md", "docs/"} {
		_, err := ParseRepo("/repo/.bark/config.toml", []byte("house_rules_files = ['"+bad+"']\n"))
		require.Error(t, err, bad)
		assert.Contains(t, err.Error(), "house_rules_files")
		assert.Contains(t, err.Error(), "/repo/.bark/config.toml")
	}
}

// The repo's preferred reviewers beat the user's own, and an empty list
// in the repo turns them off.
func TestPreferredReviewers(t *testing.T) {
	t.Parallel()

	assert.Empty(t, DefaultData().PreferredReviewers, "none by default")

	cfg := New(filepath.Join(t.TempDir(), configFileName), Data{PreferredReviewers: []string{"Yoda", "Linus"}})
	assert.Equal(t, []string{"Yoda", "Linus"}, cfg.GetPreferredReviewers())

	got := cfg.GetPreferredReviewers()
	got[0] = "changed"
	assert.Equal(t, "Yoda", cfg.GetPreferredReviewers()[0], "returns a copy")

	cfg.ApplyRepo("/repo/.bark/config.toml", RepoData{PreferredReviewers: &[]string{"Bob"}})
	assert.Equal(t, []string{"Bob"}, cfg.GetPreferredReviewers(), "the repo beats the user")

	cfg.ApplyRepo("/repo/.bark/config.toml", RepoData{PreferredReviewers: &[]string{}})
	assert.Empty(t, cfg.GetPreferredReviewers(), "an empty repo list beats the user's")
}

func TestParseRepo_PreferredReviewers(t *testing.T) {
	t.Parallel()

	got, err := ParseRepo("/repo/.bark/config.toml", []byte("preferred_reviewers = ['Yoda', 'Linus']\n"))
	require.NoError(t, err)
	assert.Equal(t, RepoData{PreferredReviewers: &[]string{"Yoda", "Linus"}}, got)

	for _, bad := range []string{"''", "'  '"} {
		_, err := ParseRepo("/repo/.bark/config.toml", []byte("preferred_reviewers = ['Yoda', "+bad+"]\n"))
		require.Error(t, err, bad)
		assert.Contains(t, err.Error(), "preferred_reviewers entry 2")
		assert.Contains(t, err.Error(), "/repo/.bark/config.toml")
	}

	// The unknown-key error lists every key a repo may set.
	_, err = ParseRepo("/repo/.bark/config.toml", []byte("editor = 'vi'\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "preferred_reviewers")
}

func TestSeveritiesAndEffortsAreCopies(t *testing.T) {
	s := Severities()
	assert.Equal(t, []string{"critical", "high", "medium", "low"}, s)
	s[0] = "changed"
	assert.Equal(t, "critical", Severities()[0])

	e := Efforts()
	assert.Equal(t, []string{"low", "medium", "high"}, e)
	e[0] = "changed"
	assert.Equal(t, "low", Efforts()[0])
}

func TestNormaliseApproveBelow(t *testing.T) {
	got, err := NormaliseApproveBelow("  HIGH ")
	require.NoError(t, err)
	assert.Equal(t, "high", got)

	got, err = NormaliseApproveBelow("")
	require.NoError(t, err)
	assert.Empty(t, got)

	_, err = NormaliseApproveBelow("severe")
	assert.EqualError(t, err, `invalid approve_below "severe" (supported: critical, high, medium, low)`)
}

func TestNormaliseLLMTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		want    string
		wantErr string
	}{
		{in: "", want: ""},
		{in: "  ", want: ""},
		{in: "10m", want: "10m"},
		{in: " 1h30m ", want: "1h30m"},
		{in: "90s", want: "90s"},
		{in: "300", wantErr: "want a duration such as 10m or 90s"},
		{in: "ten minutes", wantErr: "want a duration such as 10m or 90s"},
		{in: "0s", wantErr: "must be more than 0"},
		{in: "-5m", wantErr: "must be more than 0"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			got, err := NormaliseLLMTimeout(tt.in)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Contains(t, err.Error(), LLMTimeoutKey)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseRepo_LLMTimeout(t *testing.T) {
	t.Parallel()

	got, err := ParseRepo("/repo/.bark/config.toml", []byte("llm_timeout = ' 10m '\n"))
	require.NoError(t, err)
	assert.Equal(t, RepoData{LLMTimeout: new("10m")}, got)

	_, err = ParseRepo("/repo/.bark/config.toml", []byte("llm_timeout = '10'\n"))
	require.Error(t, err)
	for _, s := range []string{"config.toml", "llm_timeout", `"10"`} {
		assert.Contains(t, err.Error(), s)
	}
}
