package prreview

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ionut-t/bark/v2/internal/github"
	"github.com/stretchr/testify/require"
)

// testPull is the pull request the fakes answer for.
var testPull = github.Pull{Owner: "o", Name: "r", Number: 7}

// testRef is what the fakes answer to PullRef.
var testRef = github.PullRef{Pull: testPull, URL: "https://github.com/o/r/pull/7", HeadSHA: "abc123"}

// noGitHub fails every call. The fakes embed it and answer the calls
// their tests expect.
type noGitHub struct{}

var _ client = noGitHub{}

func unexpected(call string) error {
	return fmt.Errorf("unexpected call to %s", call)
}

func (noGitHub) PullRef(context.Context, string) (github.PullRef, error) {
	return github.PullRef{}, unexpected("PullRef")
}

func (noGitHub) PostReview(context.Context, github.Pull, any) (string, error) {
	return "", unexpected("PostReview")
}

func (noGitHub) DismissReview(context.Context, github.Pull, int64, string) error {
	return unexpected("DismissReview")
}

func (noGitHub) Reviews(context.Context, github.Pull) ([]github.Review, error) {
	return nil, unexpected("Reviews")
}

func (noGitHub) ReviewComments(context.Context, github.Pull) ([]github.ReviewComment, error) {
	return nil, unexpected("ReviewComments")
}

func (noGitHub) Reply(context.Context, github.Pull, int64, string) error {
	return unexpected("Reply")
}

func (noGitHub) ThumbsDown(context.Context, github.Repo, int64) ([]github.Reactor, error) {
	return nil, unexpected("ThumbsDown")
}

func (noGitHub) CanWrite(context.Context, github.Repo, string) (bool, error) {
	return false, unexpected("CanWrite")
}

func (noGitHub) ReviewThreads(context.Context, github.Pull) ([]github.ReviewThread, error) {
	return nil, unexpected("ReviewThreads")
}

func (noGitHub) ResolveThread(context.Context, string) error {
	return unexpected("ResolveThread")
}

func (noGitHub) UnresolveThread(context.Context, string) error {
	return unexpected("UnresolveThread")
}

// failingGH puts a gh on PATH that fails every call and logs its arguments
// to the returned file, so the exported functions can be checked to go
// through the real client.
func failingGH(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as gh")
	}
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	script := "#!/bin/sh\necho \"$@\" >> '" + log + "'\necho 'gh: boom' >&2\nexit 1\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755))
	t.Setenv("PATH", bin)
	return log
}

func calls(t *testing.T, log string) string {
	t.Helper()
	b, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(b)
}
