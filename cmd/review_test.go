package cmd

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPickPreferredReviewer(t *testing.T) {
	t.Parallel()

	assert.Empty(t, pickPreferredReviewer(nil, func(int) int { panic("not called") }))

	names := []string{"Yoda", "Linus", "Bob"}
	for i, want := range names {
		got := pickPreferredReviewer(names, func(n int) int {
			assert.Equal(t, len(names), n, "every name can be picked")
			return i
		})
		assert.Equal(t, want, got)
	}
}

// The review action and Bark's own workflow read the picked reviewer from
// Bark's messages for their "Reviewed by" label, so they must match the line
// Bark prints.
func TestReviewActionReadsPickedReviewer(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"../.github/actions/bark-review/action.yaml",
		"../.github/workflows/bark.yaml",
	} {
		script, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(script), "PICKED_PREFIX='"+pickedReviewerPrefix+"'", path)
	}
}

// The repository's reviewer, which beats the preferred reviewers, is found
// at the repository's root from a subdirectory too.
func TestRepoReviewerExists(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	require.NoError(t, exec.Command("git", "init", "-q").Run())
	sub := filepath.Join(root, "internal", "pkg")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	t.Chdir(sub)

	assert.False(t, repoReviewerExists(context.Background()), "no reviewer file")

	require.NoError(t, os.MkdirAll(filepath.Join(root, ".bark"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".bark", "reviewer.md"), []byte("Repo persona."), 0o644))
	assert.True(t, repoReviewerExists(context.Background()))
}

// --format takes text or json, and rejects anything else before doing any
// work; json can't be streamed.
func TestReviewFormatFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"--format", "sarif"}, want: `unknown --format "sarif": use text or json`},
		{args: []string{"--format", "json", "--stream"}, want: "can't be streamed"},
		{args: []string{"--format", "json", "--dry-run"}, want: "no JSON output"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			t.Parallel()

			cmd := reviewCmd(staticConfig(config.New("", config.Data{})))
			cmd.SetArgs(tt.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			assert.ErrorContains(t, cmd.Execute(), tt.want)
		})
	}
}

// A plain review has no menu to ask what to review, so it needs a source
// flag or a piped diff instead of quietly reviewing the uncommitted
// changes. --commit picks a commit in the TUI, so it isn't one.
func TestPlainSourceError(t *testing.T) {
	t.Parallel()

	diff := "diff --git a/a.go b/a.go\n"
	tests := []struct {
		name   string
		commit bool
		source bool
		stdin  *string
		want   string
	}{
		{name: "source flag", source: true},
		{name: "piped diff", stdin: &diff},
		{name: "nothing", want: "say what to review: -c (uncommitted changes), -s (staged), -b <branch>, --hash <commit> or --pr <number>, or pipe a diff"},
		{name: "commit picker", commit: true, source: true, want: "--commit picks a commit in the TUI; to review one in plain mode, pass --hash <commit>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := plainSourceError(tt.commit, tt.source, tt.stdin)
			if tt.want == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.want)
			}
		})
	}
}
