package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// barkBin is the path of the bark binary built once for the tests below.
var barkBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "bark-bin-")
	if err != nil {
		panic(err)
	}

	barkBin = filepath.Join(dir, "bark")
	build := exec.Command("go", "build", "-o", barkBin, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		_ = os.RemoveAll(dir)
		panic(err)
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

type barkResult struct {
	exitCode int
	stdout   string
	stderr   string
}

// runBark runs the bark binary with an isolated HOME, no TTY and no
// provider credentials, so no command can reach an LLM.
func runBark(t *testing.T, args ...string) barkResult {
	t.Helper()
	return runBarkWithStdin(t, nil, args...)
}

// runBarkWithStdin is runBark with stdin read from in (nil for none). A
// run that takes over a minute is killed, so a hang fails the test.
func runBarkWithStdin(t *testing.T, in *os.File, args ...string) barkResult {
	t.Helper()
	return runBarkIn(t, t.TempDir(), in, args...)
}

// runBarkIn is runBarkWithStdin run in dir, which is also HOME.
func runBarkIn(t *testing.T, home string, in *os.File, args ...string) barkResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, barkBin, args...)
	if in != nil {
		cmd.Stdin = in
	}
	cmd.Dir = home
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
		"TERM=dumb",
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr, "bark did not run")
		exitCode = exitErr.ExitCode()
	}

	return barkResult{exitCode: exitCode, stdout: stdout.String(), stderr: stderr.String()}
}

func TestCommandErrors_ExitNonZeroAndPrintToStderr(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "review: --with-description without --pr",
			args:    []string{"review", "--plain", "--with-description"},
			wantErr: "--with-description requires --pr",
		},
		{
			name:    "review: --full without --pr",
			args:    []string{"review", "--plain", "--full"},
			wantErr: "--full requires --pr",
		},
		{
			name:    "commit: invalid provider",
			args:    []string{"commit", "--plain", "--provider", "bogus"},
			wantErr: "error overriding provider",
		},
		{
			name:    "pr: invalid provider",
			args:    []string{"pr", "--plain", "--provider", "bogus"},
			wantErr: "error overriding provider",
		},
		{
			name:    "delete: no asset type",
			args:    []string{"delete"},
			wantErr: "must specify either reviewer or instruction to delete",
		},
		{
			name:    "delete: unknown instruction",
			args:    []string{"delete", "-i", "does-not-exist"},
			wantErr: "error deleting instruction",
		},
		{
			name:    "edit: no asset type",
			args:    []string{"edit"},
			wantErr: "must specify either commit, reviewer or instruction to edit",
		},
		{
			name:    "reset: nothing to reset",
			args:    []string{"reset"},
			wantErr: "specify at least one of --reviewers or --instructions to reset",
		},
		{
			name:    "add: missing argument (cobra error)",
			args:    []string{"add"},
			wantErr: "accepts 1 arg(s), received 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := runBark(t, tt.args...)

			assert.Equal(t, 1, res.exitCode, "exit code\nstdout: %s\nstderr: %s", res.stdout, res.stderr)
			assert.Contains(t, res.stderr, "Error: "+tt.wantErr)
			assert.NotContains(t, res.stdout, tt.wantErr, "error must not be printed to stdout")
			assert.NotContains(t, res.stderr, "\x1b[", "stderr is not a terminal, so it must have no ANSI codes")
		})
	}
}

func TestCommandErrors_PrintedOnce(t *testing.T) {
	res := runBark(t, "reset")

	assert.Equal(t, 1, bytes.Count([]byte(res.stdout+res.stderr), []byte("specify at least one of")),
		"error printed more than once\nstdout: %s\nstderr: %s", res.stdout, res.stderr)
}

func TestCommandSuccess_ExitZeroAndPrintToStdout(t *testing.T) {
	res := runBark(t, "reset", "--instructions")

	assert.Equal(t, 0, res.exitCode, "stderr: %s", res.stderr)
	assert.Contains(t, res.stdout, "Instructions have been reset to default")
	assert.NotContains(t, res.stderr, "Error")
}

func TestConfigFlag_UsesGivenFile(t *testing.T) {
	tests := []struct {
		name string
		args func(path string) []string
	}{
		{name: "before the subcommand", args: func(p string) []string { return []string{"--config", p, "config", "-m", "flag-model"} }},
		{name: "after the subcommand", args: func(p string) []string { return []string{"config", "--config", p, "-m", "flag-model"} }},
		{name: "with =", args: func(p string) []string { return []string{"config", "--config=" + p, "-m", "flag-model"} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "custom.toml")
			require.NoError(t, os.WriteFile(path, []byte("editor = 'true'\nllm_model = 'old-model'\n"), 0o644))

			res := runBark(t, tt.args(path)...)

			require.Equal(t, 0, res.exitCode, "stderr: %s", res.stderr)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Contains(t, string(data), "flag-model", "model must be saved to the --config file")
		})
	}
}

func TestConfigFlag_AcceptedByEverySubcommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.toml")
	require.NoError(t, os.WriteFile(path, []byte("editor = 'true'\n"), 0o644))

	for _, sub := range []string{"config", "review", "commit", "pr", "reset", "add", "delete", "edit", "actions"} {
		t.Run(sub, func(t *testing.T) {
			res := runBark(t, sub, "--config", path, "--help")

			assert.Equal(t, 0, res.exitCode, "stderr: %s", res.stderr)
			assert.NotContains(t, res.stderr, "unknown flag")
		})
	}
}

func TestConfigFlag_MissingFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.toml")

	res := runBark(t, "config", "--config", path, "-m", "flag-model")

	assert.Equal(t, 1, res.exitCode, "stdout: %s\nstderr: %s", res.stdout, res.stderr)
	assert.Contains(t, res.stderr, "Error: ")
	assert.NoFileExists(t, path, "a mistyped --config path must not be created")
}

// A flag that names the diff's source means stdin isn't read, so a pipe an
// agent or task runner leaves open doesn't block the run (bug 31). The
// directory isn't a git repository, so each run then fails on git.
func TestSourceFlag_DoesNotWaitOnOpenStdin(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"review --branch", []string{"review", "--plain", "--dry-run", "--branch", "main"}},
		{"review --staged", []string{"review", "--plain", "--dry-run", "--staged"}},
		{"review --hash", []string{"review", "--plain", "--dry-run", "--hash", "HEAD"}},
		{"pr --branch", []string{"pr", "--plain", "--branch", "main"}},
		{"commit --all", []string{"commit", "--plain", "--all"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			require.NoError(t, err)
			t.Cleanup(func() { _ = r.Close(); _ = w.Close() }) // w stays open during the run

			start := time.Now()
			res := runBarkWithStdin(t, r, tt.args...)

			assert.Less(t, time.Since(start), 30*time.Second, "bark waited on stdin")
			assert.Equal(t, 1, res.exitCode, "stdout: %s\nstderr: %s", res.stdout, res.stderr)
			assert.NotContains(t, res.stderr, "waiting for a diff on stdin")
		})
	}
}

// A repository's .bark/config.toml can set preferred_reviewers, so a name
// picked from it is only looked up among the reviewers: a path there must
// not read a local file into the prompt.
func TestPreferredReviewers_PathNotRead(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "private.md")
	require.NoError(t, os.WriteFile(secret, []byte("PRIVATE-NOTES-CONTENT"), 0o644))

	repo := t.TempDir()
	require.NoError(t, exec.Command("git", "-C", repo, "init", "-q").Run())
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".bark"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".bark", "config.toml"),
		[]byte("preferred_reviewers = ['"+secret+"']\n"), 0o644))
	cfg := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(cfg, []byte("editor = 'true'\nllm_provider = 'openai'\n"), 0o644))

	diff := filepath.Join(t.TempDir(), "change.diff")
	require.NoError(t, os.WriteFile(diff, []byte("diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-a\n+b\n"), 0o644))
	in, err := os.Open(diff)
	require.NoError(t, err)
	t.Cleanup(func() { _ = in.Close() })

	res := runBarkIn(t, repo, in, "review", "--plain", "--dry-run", "--skip-instruction", "--config", cfg)

	assert.NotContains(t, res.stdout+res.stderr, "PRIVATE-NOTES-CONTENT")
	assert.Equal(t, 1, res.exitCode, "stdout: %s\nstderr: %s", res.stdout, res.stderr)
	assert.Contains(t, res.stderr, "reviewer not found")
}
