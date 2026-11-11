package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeRepoConfig writes a repository config under a new temp root and
// returns the root.
func writeRepoConfig(t *testing.T, content string) string {
	t.Helper()
	root := t.TempDir()
	path := config.RepoFilePath(root)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return root
}

func TestApplyRepoConfig(t *testing.T) {
	t.Parallel()

	root := writeRepoConfig(t, "llm_model = 'claude-sonnet-5-5'\nmin_severity = 'high'\n")
	cfg := config.New("", config.Data{LLMModel: "gemini-2.5-pro", LLMProvider: "gemini"})

	require.NoError(t, applyRepoConfig(cfg, root))

	model, _ := cfg.GetLLMModel()
	provider, _ := cfg.GetLLMProvider()
	assert.Equal(t, "claude-sonnet-5-5", model)
	assert.Equal(t, "gemini", provider, "keys the repository doesn't set keep the user's value")
	assert.Equal(t, "high", cfg.GetMinSeverity())
}

func TestApplyRepoConfig_NoFile(t *testing.T) {
	t.Parallel()

	cfg := config.New("", config.Data{LLMModel: "gemini-2.5-pro"})
	require.NoError(t, applyRepoConfig(cfg, t.TempDir()))

	model, _ := cfg.GetLLMModel()
	assert.Equal(t, "gemini-2.5-pro", model)
	assert.Empty(t, cfg.RepoLLMWarning())
}

func TestApplyRepoConfig_InvalidFile(t *testing.T) {
	t.Parallel()

	root := writeRepoConfig(t, "editor = 'sh -c evil'\n")
	cfg := config.New("", config.Data{Editor: "vi"})

	err := applyRepoConfig(cfg, root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), config.RepoFilePath(root))
	assert.Equal(t, "vi", cfg.GetEditor())
}

func TestWarnRepoLLM(t *testing.T) {
	t.Parallel()

	root := writeRepoConfig(t, "llm_provider = 'anthropic'\nllm_model = 'claude-opus-5-5'\n")
	cfg := config.New("", config.Data{LLMProvider: "gemini", LLMModel: "gemini-2.5-pro"})
	require.NoError(t, applyRepoConfig(cfg, root))

	var out bytes.Buffer
	warnRepoLLM(&out, cfg)
	assert.Contains(t, out.String(), "bark: warning: ")
	assert.Contains(t, out.String(), `llm_model = "claude-opus-5-5"`)

	cfg.OverrideModel("gemini-3.6-flash")
	require.NoError(t, cfg.OverrideProvider("gemini"))
	out.Reset()
	warnRepoLLM(&out, cfg)
	assert.Empty(t, out.String(), "no warning once flags choose both")
}
