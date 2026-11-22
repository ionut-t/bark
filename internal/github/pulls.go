package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// A pull request is named the way gh accepts it: a number, a URL or a
// branch.

// PullRef says where a pull request is.
type PullRef struct {
	Pull
	URL string
	// HeadSHA is the commit the pull request's branch points to.
	HeadSHA string
}

// PullRef looks up pull request pr.
func (c *Client) PullRef(ctx context.Context, pr string) (PullRef, error) {
	out, err := c.run(ctx, nil, "pr", "view", pr, "--json", "url,headRefOid")
	if err != nil {
		return PullRef{}, err
	}
	var raw struct {
		URL     string `json:"url"`
		HeadSHA string `json:"headRefOid"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return PullRef{}, fmt.Errorf("failed to parse pull request %s: %w", pr, err)
	}
	pull, err := ParsePullURL(raw.URL)
	if err != nil {
		return PullRef{}, err
	}
	return PullRef{Pull: pull, URL: raw.URL, HeadSHA: raw.HeadSHA}, nil
}

// PullRequest is what a pull request says about itself.
type PullRequest struct {
	Number int
	Title  string
	Body   string
	// HeadSHA is the commit the pull request's branch points to.
	HeadSHA string
	// BaseSHA is the commit the branch it merges into points to.
	BaseSHA string
	Commits []Commit
	// Files are the files it changes, as GitHub lists them: it may list
	// fewer than ChangedFiles.
	Files        []File
	ChangedFiles int
}

// Commit is a commit of a pull request.
type Commit struct {
	Headline string
	Body     string
}

// File is a file a pull request changes.
type File struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// PullRequest reads pull request pr.
func (c *Client) PullRequest(ctx context.Context, pr string) (PullRequest, error) {
	out, err := c.run(ctx, nil, "pr", "view", pr, "--json", "commits,title,number,body,headRefOid,baseRefOid,files,changedFiles")
	if err != nil {
		return PullRequest{}, err
	}
	var raw struct {
		Number       int    `json:"number"`
		Title        string `json:"title"`
		Body         string `json:"body"`
		HeadSHA      string `json:"headRefOid"`
		BaseSHA      string `json:"baseRefOid"`
		Files        []File `json:"files"`
		ChangedFiles int    `json:"changedFiles"`
		Commits      []struct {
			MessageHeadline string `json:"messageHeadline"`
			MessageBody     string `json:"messageBody"`
		} `json:"commits"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return PullRequest{}, fmt.Errorf("failed to parse PR metadata: %w", err)
	}
	commits := make([]Commit, len(raw.Commits))
	for i, c := range raw.Commits {
		commits[i] = Commit{Headline: c.MessageHeadline, Body: c.MessageBody}
	}
	return PullRequest{
		Number:       raw.Number,
		Title:        raw.Title,
		Body:         raw.Body,
		HeadSHA:      raw.HeadSHA,
		BaseSHA:      raw.BaseSHA,
		Commits:      commits,
		Files:        raw.Files,
		ChangedFiles: raw.ChangedFiles,
	}, nil
}

// PullRequestDiff returns the unified diff of pull request pr.
func (c *Client) PullRequestDiff(ctx context.Context, pr string) (string, error) {
	out, err := c.run(ctx, nil, "pr", "diff", pr)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// FileContent reads path at commit sha from the current repository.
func (c *Client) FileContent(ctx context.Context, sha, path string) ([]byte, error) {
	endpoint := "repos/{owner}/{repo}/contents/" + (&url.URL{Path: path}).EscapedPath() + "?ref=" + url.QueryEscape(sha)
	return c.run(ctx, nil, "api", "-H", "Accept: application/vnd.github.raw", endpoint)
}
