// Package github is Bark's only way to talk to GitHub. It runs the gh CLI,
// which brings its own authentication (gh auth login, GH_TOKEN, GitHub
// Enterprise hosts, GITHUB_TOKEN in Actions) and works out the current
// repository from the git remotes.
//
// Its methods are named after what they do on GitHub, so callers don't
// build gh command lines, API paths or jq filters themselves.
package github

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
)

// ErrNotInstalled is returned when the gh CLI isn't on PATH.
var ErrNotInstalled = errors.New("gh CLI is not installed (see https://cli.github.com)")

// runner runs the gh CLI with args, feeding it stdin if not nil, and
// returns its stdout. Tests replace it.
type runner func(ctx context.Context, stdin []byte, args ...string) ([]byte, error)

// Client talks to GitHub through the gh CLI.
type Client struct {
	run runner
}

// New returns a Client that runs the gh CLI on PATH.
func New() *Client {
	return &Client{run: runGH}
}

func runGH(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, ErrNotInstalled
		}
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			// gh api prints the response body on stdout and the status on stderr.
			if msg := strings.TrimSpace(string(exitErr.Stderr) + "\n" + string(out)); msg != "" {
				return nil, fmt.Errorf("%w: %s", err, msg)
			}
		}
		return nil, err
	}
	return out, nil
}

// Repo is a repository on GitHub.
type Repo struct {
	Owner string
	Name  string
}

// path is the repository's API path, such as repos/owner/repo.
func (r Repo) path() string {
	return "repos/" + r.Owner + "/" + r.Name
}

// Pull is a pull request.
type Pull struct {
	Repo
	Number int
}

// path is the pull request's API path, such as repos/owner/repo/pulls/12.
func (p Pull) path() string {
	return fmt.Sprintf("%s/pulls/%d", p.Repo.path(), p.Number)
}

// ParsePullURL returns the pull request at a URL such as
// https://github.com/owner/repo/pull/12. Taking the repository from the
// URL, rather than gh's {owner}/{repo}, keeps it right when the pull
// request is in another repository than the current one.
func ParsePullURL(prURL string) (Pull, error) {
	u, err := url.Parse(prURL)
	if err == nil {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) == 4 && parts[2] == "pull" && parts[0] != "" && parts[1] != "" {
			if n, err := strconv.Atoi(parts[3]); err == nil && n > 0 {
				return Pull{Owner: parts[0], Name: parts[1], Number: n}, nil
			}
		}
	}
	return Pull{}, fmt.Errorf("unexpected pull request URL %q", prURL)
}
