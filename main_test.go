package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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

	home := t.TempDir()
	cmd := exec.Command(barkBin, args...)
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
