package check

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// promptLLM records the prompts it's sent and keeps every finding.
type promptLLM struct{ prompts *[]string }

func (p promptLLM) Stream(context.Context, string, string) (<-chan llm.Response, <-chan error) {
	panic("the check doesn't stream")
}

func (p promptLLM) Generate(_ context.Context, _, prompt string) (llm.Response, error) {
	*p.prompts = append(*p.prompts, prompt)
	return llm.Response{Content: `{"id":1,"verdict":"keep"}`, Usage: &llm.Usage{OutputTokens: 1}}, nil
}

// The file a finding names comes from the model, so the check must read
// only files the diff changes: a "../" path written in by a prompt
// injection must not send a file from outside the repository to the
// provider.
func TestReviewRunReadsOnlyFilesInTheDiff(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	base := t.TempDir()
	dir := filepath.Join(base, "repo")
	require.NoError(t, os.Mkdir(dir, 0o755))
	const secret = "aws_secret_access_key = SECRET-OUTSIDE-REPO"
	require.NoError(t, os.WriteFile(filepath.Join(base, "credentials"), []byte(secret+"\n"), 0o644))
	const tracked = "package a\n\nconst Untouched = \"NOT-IN-DIFF\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.go"), []byte(tracked), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644))
	for _, args := range [][]string{{"init", "-b", "main"}, {"add", "."}, {"commit", "-m", "base"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nvar Changed = 1\n"), 0o644))
	t.Chdir(dir)

	rd, err := git.GetReviewDiff(t.Context(), git.WorkingTreeDiff(false))
	require.NoError(t, err)

	events := []findings.Event{
		{Finding: &findings.Finding{File: "../credentials", Line: 1, Severity: findings.High, Title: "Leaked key"}},
		{Finding: &findings.Finding{File: "other.go", Line: 3, Severity: findings.Low, Title: "Not in the diff"}},
		{Finding: &findings.Finding{File: "a.go", Line: 3, Severity: findings.Low, Title: "In the diff"}},
	}
	var prompts []string
	r := Review{Client: promptLLM{&prompts}, ReviewDiff: rd, Diff: rd.Diff}
	_, err = r.Run(t.Context(), events)
	require.NoError(t, err)

	require.Len(t, prompts, 1)
	assert.NotContains(t, prompts[0], "SECRET-OUTSIDE-REPO", "a file outside the repository was sent")
	assert.NotContains(t, prompts[0], "NOT-IN-DIFF", "a file outside the diff was sent")
	assert.Contains(t, prompts[0], "var Changed = 1", "the changed file is still sent")
}
