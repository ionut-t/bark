package prreview

import (
	"context"
	"fmt"

	"github.com/ionut-t/bark/v2/internal/github"
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
