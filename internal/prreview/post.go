package prreview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/git"
)

// gh runs the gh CLI with stdin and returns its stdout. Tests replace it.
type gh func(ctx context.Context, stdin []byte, args ...string) ([]byte, error)

// Result says how a review was posted.
type Result struct {
	URL string
	// Inline is the number of findings posted as inline comments.
	Inline int
	// Fallback is true when GitHub rejected the inline comments and every
	// finding was posted in the review body instead.
	Fallback bool
}

// Post posts the events of a structured review of diff to pull request pr
// (a number, URL or branch, as gh accepts) as a review with inline
// comments. diff must be the diff the review was made from, so that only
// lines it shows get comments.
//
// If GitHub rejects the review (for example because the pull request moved
// on since diff was fetched, so a line is no longer in it), Post posts it
// again with every finding in the body.
func Post(ctx context.Context, pr string, events []findings.Event, diff string) (Result, error) {
	return post(ctx, runGH, pr, events, diff)
}

func post(ctx context.Context, run gh, pr string, events []findings.Event, diff string) (Result, error) {
	out, err := run(ctx, nil, "pr", "view", pr, "--json", "url,headRefOid")
	if err != nil {
		return Result{}, fmt.Errorf("failed to look up pull request %s: %w", pr, err)
	}
	var meta struct {
		URL        string `json:"url"`
		HeadRefOid string `json:"headRefOid"`
	}
	if err := json.Unmarshal(out, &meta); err != nil {
		return Result{}, fmt.Errorf("failed to parse pull request %s: %w", pr, err)
	}
	endpoint, err := reviewsEndpoint(meta.URL)
	if err != nil {
		return Result{}, err
	}

	review := Build(events, diff)
	review.CommitID = meta.HeadRefOid
	result, err := send(ctx, run, endpoint, review)
	if err == nil || len(review.Comments) == 0 {
		return result, err
	}

	review = Build(events, "")
	review.CommitID = meta.HeadRefOid
	result, retryErr := send(ctx, run, endpoint, review)
	if retryErr != nil {
		return Result{}, fmt.Errorf("failed to post review: %w", errors.Join(err, retryErr))
	}
	result.Fallback = true
	return result, nil
}

// Earlier returns the findings that reviews Bark posted earlier on pull
// request pr (a number, URL or branch, as gh accepts) recorded, oldest
// first: the points already raised there. Reviews posted before Bark
// recorded findings add none.
func Earlier(ctx context.Context, pr string) ([]findings.Finding, error) {
	return earlier(ctx, runGH, pr)
}

// earlierReviewsQuery picks the bodies of Bark's reviews, each as a JSON
// string on a line of its own.
const earlierReviewsQuery = `.[] | select(.body | startswith("` + Marker + `")) | .body | @json`

func earlier(ctx context.Context, run gh, pr string) ([]findings.Finding, error) {
	out, err := run(ctx, nil, "pr", "view", pr, "--json", "url")
	if err != nil {
		return nil, fmt.Errorf("failed to look up pull request %s: %w", pr, err)
	}
	var meta struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(out, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse pull request %s: %w", pr, err)
	}
	endpoint, err := reviewsEndpoint(meta.URL)
	if err != nil {
		return nil, err
	}

	out, err = run(ctx, nil, "api", "--paginate", endpoint, "--jq", earlierReviewsQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to read earlier reviews: %w", err)
	}
	var fs []findings.Finding
	for line := range strings.SplitSeq(string(out), "\n") {
		var body string
		if json.Unmarshal([]byte(line), &body) == nil {
			fs = append(fs, Recorded(body)...)
		}
	}
	return fs, nil
}

func send(ctx context.Context, run gh, endpoint string, review Review) (Result, error) {
	payload, err := json.Marshal(review)
	if err != nil {
		return Result{}, err
	}
	out, err := run(ctx, payload, "api", "--method", "POST", endpoint, "--input", "-")
	if err != nil {
		return Result{}, fmt.Errorf("failed to post review: %w", err)
	}

	var created struct {
		HTMLURL string `json:"html_url"`
	}
	_ = json.Unmarshal(out, &created)
	return Result{URL: created.HTMLURL, Inline: len(review.Comments)}, nil
}

// reviewsEndpoint returns the reviews API path for a pull request URL such
// as https://github.com/owner/repo/pull/12. Taking the repository from the
// URL, rather than gh's {owner}/{repo}, keeps it right when pr is a URL to
// another repository.
func reviewsEndpoint(prURL string) (string, error) {
	u, err := url.Parse(prURL)
	if err == nil {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) == 4 && parts[2] == "pull" {
			return fmt.Sprintf("repos/%s/%s/pulls/%s/reviews", parts[0], parts[1], parts[3]), nil
		}
	}
	return "", fmt.Errorf("unexpected pull request URL %q", prURL)
}

func runGH(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, git.ErrGHNotInstalled
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
