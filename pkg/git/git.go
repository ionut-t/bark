package git

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var (
	ErrNotAGitRepository     = errors.New("not a git repository")
	ErrNoChangesInRepository = errors.New("no changes in repository")
	ErrNoCommitsInRepository = errors.New("no commits in repository")
	ErrGHNotInstalled        = errors.New("gh CLI is not installed (see https://cli.github.com)")
)

var shortStatRegex = regexp.MustCompile(`(?P<files>\d+) files? changed(?:, (?P<additions>\d+) insertions?\(\+\))?(?:, (?P<deletions>\d+) deletions?\(-\))?`)

// defaultDiffExcludes are pathspecs applied to every diff to filter out generated and
// dependency files that add token cost without useful review signal.
var defaultDiffExcludes = []string{
	":(exclude)*.sum",
	":(exclude)*.lock",
	":(exclude)*.pb.go",
	":(exclude)*.pb.gw.go",
}

// Commit represents a single git commit.
type Commit struct {
	Hash    string
	Author  string
	Date    string
	Message string
	Body    string
}

// BranchInfo contains information about a branch for PR description
type BranchInfo struct {
	Name              string
	BaseBranch        string
	Commits           []Commit
	TotalFilesChanged int
	TotalAdditions    int
	TotalDeletions    int
	Diffs             string
}

// IsGitRepo checks if the current directory is a git repository.
func IsGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	output, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(output)) == "true"
}

// GetCommits returns a list of the most recent commits.
func GetCommits(ctx context.Context, limit int) ([]Commit, error) {
	cmd := exec.CommandContext(ctx, "git", "log", "--pretty=format:%H|%an|%ar|%s", "-n", strconv.Itoa(limit))

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			// Check if the error is due to no commits in the repository
			if exitErr.ExitCode() == 128 {
				return nil, ErrNoCommitsInRepository
			}
		}

		return nil, fmt.Errorf("failed to get commits: %w", err)
	}

	var commits []Commit
	for line := range strings.SplitSeq(string(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) == 4 {
			commits = append(commits, Commit{
				Hash:    parts[0],
				Author:  parts[1],
				Date:    parts[2],
				Message: parts[3],
			})
		}
	}

	return commits, nil
}

// GetCommit returns a single commit.
func GetCommit(ctx context.Context, hash string) (*Commit, error) {
	cmd := exec.CommandContext(ctx, "git", "show", "--pretty=format:%H|%an|%ar|%s", "-s", hash)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get commit %s: %w", hash, err)
	}

	line := strings.TrimSpace(string(output))
	parts := strings.SplitN(line, "|", 4)
	if len(parts) == 4 {
		return &Commit{
			Hash:    parts[0],
			Author:  parts[1],
			Date:    parts[2],
			Message: parts[3],
		}, nil
	}

	return nil, nil
}

// GetDiff returns the diff for a given commit hash.
func GetDiff(ctx context.Context, hash string) (string, error) {
	if !IsGitRepo() {
		return "", ErrNotAGitRepository
	}

	args := append([]string{"show", hash, "--"}, defaultDiffExcludes...)
	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get diff for commit %s: %w", hash, err)
	}

	return string(output), nil
}

// GetWorkingTreeDiff returns the current uncommitted changes in the working directory.
func GetWorkingTreeDiff(ctx context.Context, all bool) (string, error) {
	if !IsGitRepo() {
		return "", ErrNotAGitRepository
	}

	var baseArgs []string
	if all {
		baseArgs = []string{"diff", "HEAD", "--"}
	} else {
		baseArgs = []string{"diff", "--staged", "--"}
	}
	cmd := exec.CommandContext(ctx, "git", append(baseArgs, defaultDiffExcludes...)...)

	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && all {
			// Check if the error is due to no changes in the working directory
			if exitErr.ExitCode() == 128 {
				return "", ErrNoChangesInRepository
			}
		}

		return "", fmt.Errorf("failed to get working tree diff: %w", err)
	}

	return string(output), nil
}

// GetWorkingTreeStat returns the --stat summary for uncommitted working tree changes.
// Errors are treated as best-effort: a non-nil error returns an empty string.
func GetWorkingTreeStat(ctx context.Context, all bool) string {
	var args []string
	if all {
		args = append([]string{"diff", "--stat", "HEAD", "--"}, defaultDiffExcludes...)
	} else {
		args = append([]string{"diff", "--stat", "--staged", "--"}, defaultDiffExcludes...)
	}
	out, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// GetBranchDiffStat returns the --stat summary for the diff between the current branch and branch.
func GetBranchDiffStat(ctx context.Context, branch string) string {
	args := append([]string{"diff", "--stat", branch, "--"}, defaultDiffExcludes...)
	out, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// GetCommitStat returns the --stat summary for a single commit.
func GetCommitStat(ctx context.Context, hash string) string {
	args := append([]string{"show", "--stat", hash, "--"}, defaultDiffExcludes...)
	out, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func GetCurrentBranch(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

func GetBranchDiff(ctx context.Context, branch string, maxLines uint32) (string, error) {
	args := append([]string{"diff", branch, "--"}, defaultDiffExcludes...)
	cmd := exec.CommandContext(ctx, "git", args...)

	if maxLines == 0 {
		output, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("failed to get branch diff: %w", err)
		}
		return string(output), nil
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to get branch diff: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to get branch diff: %w", err)
	}

	var sb strings.Builder
	truncated := false
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for lineCount := uint32(0); scanner.Scan(); lineCount++ {
		if lineCount >= maxLines {
			truncated = true
			break
		}
		sb.WriteString(scanner.Text())
		sb.WriteByte('\n')
	}

	_ = stdout.Close()

	if scanErr := scanner.Err(); scanErr != nil && !truncated {
		_ = cmd.Wait()
		return "", fmt.Errorf("failed to read branch diff: %w", scanErr)
	}

	if waitErr := cmd.Wait(); waitErr != nil && !truncated {
		return "", fmt.Errorf("failed to get branch diff: %w", waitErr)
	}

	if truncated {
		sb.WriteString("... (truncated)\n")
	}

	return sb.String(), nil
}

func CommitChanges(ctx context.Context, message string, all bool) (<-chan string, <-chan error) {
	outChan := make(chan string)
	errChan := make(chan error, 1)

	go func() {
		defer close(outChan)
		defer close(errChan)

		if all {
			cmd := exec.CommandContext(ctx, "git", "add", "-A")

			if out, err := cmd.CombinedOutput(); err != nil {
				errChan <- fmt.Errorf("failed to stage changes: %w\n\n %s", err, string(out))
				return
			}
		}

		reader, writer, err := os.Pipe()
		if err != nil {
			errChan <- err
			return
		}

		cmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
		cmd.Stdout = writer
		cmd.Stderr = writer

		if err := cmd.Start(); err != nil {
			_ = writer.Close()
			_ = reader.Close()
			errChan <- err
			return
		}

		if err := writer.Close(); err != nil {
			_ = reader.Close()
			errChan <- err
			return
		}

		waitDone := make(chan error, 1)
		go func() {
			waitDone <- cmd.Wait()
		}()

		var lines []string
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Text()
			lines = append(lines, line)
			outChan <- line
		}

		_ = reader.Close()

		err = <-waitDone
		if err != nil {
			errChan <- fmt.Errorf("failed to commit changes: %w\n\n%s", err, strings.Join(lines, "\n"))
		}
		if scanErr := scanner.Err(); scanErr != nil {
			errChan <- fmt.Errorf("failed to read commit output: %w", scanErr)
		}
	}()

	return outChan, errChan
}

// GetBaseBranch attempts to determine the base branch (main, master, develop)
func GetBaseBranch(ctx context.Context) (string, error) {
	// Try to find the default branch from remote
	cmd := exec.CommandContext(ctx, "git", "symbolic-ref", "refs/remotes/origin/HEAD")
	output, err := cmd.Output()
	if err == nil {
		parts := strings.Split(strings.TrimSpace(string(output)), "/")
		if len(parts) > 0 {
			return parts[len(parts)-1], nil
		}
	}

	return "", fmt.Errorf("could not determine base branch")
}

// GetBranchCommits gets all commits on current branch that aren't in base branch
func GetBranchCommits(ctx context.Context, baseBranch string) ([]Commit, error) {
	currentBranch, err := GetCurrentBranch(ctx)
	if err != nil {
		return nil, err
	}

	// Get commits that are in current branch but not in base
	// Format: %H = hash, %an = author, %ar = date relative, %s = subject, %b = body
	cmd := exec.CommandContext(ctx, "git", "log",
		fmt.Sprintf("%s..%s", baseBranch, currentBranch),
		"--pretty=format:%H|%an|%ar|%s|%b||END||")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get branch commits: %w", err)
	}

	if strings.TrimSpace(string(output)) == "" {
		return nil, fmt.Errorf("no commits found on branch %s", currentBranch)
	}

	var commits []Commit
	// Split by our custom delimiter
	entries := strings.SplitSeq(string(output), "||END||")
	for entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		parts := strings.SplitN(entry, "|", 5)
		if len(parts) >= 4 {
			commit := Commit{
				Hash:    parts[0],
				Author:  parts[1],
				Date:    parts[2],
				Message: parts[3],
			}
			// Add body if it exists (5th part)
			if len(parts) == 5 {
				commit.Body = strings.TrimSpace(parts[4])
			}
			commits = append(commits, commit)
		}
	}

	return commits, nil
}

// GetBranchStats gets addition/deletion stats for the branch compared to base
func GetBranchStats(ctx context.Context, baseBranch string) (filesChanged, additions, deletions int, err error) {
	currentBranch, err := GetCurrentBranch(ctx)
	if err != nil {
		return 0, 0, 0, err
	}

	cmd := exec.CommandContext(
		ctx,
		"git", "diff", "--shortstat",
		fmt.Sprintf("%s...%s", baseBranch, currentBranch),
	)
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to get branch stats: %w", err)
	}

	// Parse output like: "5 files changed, 123 insertions(+), 45 deletions(-)"
	statsLine := strings.TrimSpace(string(output))
	if statsLine == "" {
		return 0, 0, 0, nil // No changes
	}

	matches := shortStatRegex.FindStringSubmatch(statsLine)
	if len(matches) == 0 {
		return 0, 0, 0, fmt.Errorf("failed to parse git diff --shortstat output: %q", statsLine)
	}
	// Get named submatch indices for clarity
	filesIdx := shortStatRegex.SubexpIndex("files")
	additionsIdx := shortStatRegex.SubexpIndex("additions")
	deletionsIdx := shortStatRegex.SubexpIndex("deletions")

	// Extract filesChanged
	if filesIdx != -1 && matches[filesIdx] != "" {
		filesChanged, err = strconv.Atoi(matches[filesIdx])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("failed to parse files changed count '%s': %w", matches[filesIdx], err)
		}
	}

	// Extract additions
	if additionsIdx != -1 && matches[additionsIdx] != "" {
		additions, err = strconv.Atoi(matches[additionsIdx])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("failed to parse additions count '%s': %w", matches[additionsIdx], err)
		}
	}
	// Extract deletions
	if deletionsIdx != -1 && matches[deletionsIdx] != "" {
		deletions, err = strconv.Atoi(matches[deletionsIdx])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("failed to parse deletions count '%s': %w", matches[deletionsIdx], err)
		}
	}
	return filesChanged, additions, deletions, nil
}

// GetBranchInfo gets comprehensive info about the current branch for PR description
func GetBranchInfo(ctx context.Context, baseBranch string, maxLines uint32) (*BranchInfo, error) {
	currentBranch, err := GetCurrentBranch(ctx)
	if err != nil {
		return nil, err
	}

	if baseBranch == "" {
		baseBranch, err = GetBaseBranch(ctx)
		if err != nil {
			return nil, err
		}
	}

	commits, err := GetBranchCommits(ctx, baseBranch)
	if err != nil {
		return nil, err
	}

	if len(commits) == 0 {
		return nil, fmt.Errorf("no commits found on branch %s", currentBranch)
	}

	diffs, err := GetBranchDiff(ctx, baseBranch, maxLines)
	if err != nil {
		return nil, err
	}

	filesChanged, additions, deletions, err := GetBranchStats(ctx, baseBranch)
	if err != nil {
		return nil, err
	}

	return &BranchInfo{
		Name:              currentBranch,
		BaseBranch:        baseBranch,
		Commits:           commits,
		TotalFilesChanged: filesChanged,
		TotalAdditions:    additions,
		TotalDeletions:    deletions,
		Diffs:             diffs,
	}, nil
}

// GetPRDiff returns the diff for a GitHub pull request using the gh CLI.
func GetPRDiff(ctx context.Context, prNumber string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", "pr", "diff", prNumber)
	output, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrGHNotInstalled
		}

		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			if stderr := strings.TrimSpace(string(exitErr.Stderr)); stderr != "" {
				return "", fmt.Errorf("%w:\n%s", exitErr, stderr)
			}
		}

		return "", fmt.Errorf("failed to get PR diff: %w", err)
	}

	return filterDiff(string(output), defaultDiffExcludes), nil
}

var diffFileHeaderRe = regexp.MustCompile(`^diff --git a/.+ b/(.+)$`)

// filterDiff removes file sections from a unified git diff whose filename matches
// any of the given exclude patterns (:(exclude)<glob> format).
func filterDiff(diff string, excludePatterns []string) string {
	globs := make([]string, len(excludePatterns))
	for i, p := range excludePatterns {
		globs[i] = strings.TrimPrefix(p, ":(exclude)")
	}

	lines := strings.Split(diff, "\n")
	out := make([]string, 0, len(lines))
	skip := false

	for _, line := range lines {
		if m := diffFileHeaderRe.FindStringSubmatch(line); m != nil {
			filePath := m[1]
			base := path.Base(filePath)
			skip = false
			for _, glob := range globs {
				if matched, _ := path.Match(glob, base); matched {
					skip = true
					break
				}
			}
		}
		if !skip {
			out = append(out, line)
		}
	}

	return strings.Join(out, "\n")
}

// PRMeta holds lightweight metadata about a GitHub pull request.
type PRMeta struct {
	Number  int
	Title   string
	Body    string
	Commits []Commit
}

// GetPRMeta fetches the title and commit messages for a GitHub pull request via the gh CLI.
func GetPRMeta(ctx context.Context, prNumber string) (*PRMeta, error) {
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", prNumber, "--json", "commits,title,number,body")
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, ErrGHNotInstalled
		}
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			if stderr := strings.TrimSpace(string(exitErr.Stderr)); stderr != "" {
				return nil, fmt.Errorf("%s", stderr)
			}
		}
		return nil, fmt.Errorf("failed to get PR metadata: %w", err)
	}

	var raw struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		Body    string `json:"body"`
		Commits []struct {
			MessageHeadline string `json:"messageHeadline"`
			MessageBody     string `json:"messageBody"`
		} `json:"commits"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse PR metadata: %w", err)
	}

	commits := make([]Commit, len(raw.Commits))
	for i, c := range raw.Commits {
		commits[i] = Commit{Message: c.MessageHeadline, Body: c.MessageBody}
	}

	return &PRMeta{Number: raw.Number, Title: raw.Title, Body: raw.Body, Commits: commits}, nil
}

// ReviewDiffParams controls which diff GetReviewDiff fetches.
type ReviewDiffParams struct {
	pr         string
	branch     string
	maxLines   uint32
	commitHash string
	stagedOnly bool
	withBody   bool
}

func PRDiff(prNumber string) ReviewDiffParams {
	return ReviewDiffParams{pr: prNumber}
}

func BranchDiff(branch string) ReviewDiffParams {
	return ReviewDiffParams{branch: branch}
}

// WithMaxLines returns a copy of the params with a line-count limit applied to the diff.
// For branch diffs the limit is enforced during streaming; for all other sources it is
// applied as a post-processing step inside GetReviewDiff.
func (p ReviewDiffParams) WithMaxLines(n uint32) ReviewDiffParams {
	p.maxLines = n
	return p
}

// WithPRDescription includes the PR body in the review context.
// Only has effect when used with PRDiff.
func (p ReviewDiffParams) WithPRDescription() ReviewDiffParams {
	p.withBody = true
	return p
}

func CommitDiff(hash string) ReviewDiffParams {
	return ReviewDiffParams{commitHash: hash}
}

func WorkingTreeDiff(stagedOnly bool) ReviewDiffParams {
	return ReviewDiffParams{stagedOnly: stagedOnly}
}

// BranchDiffError is returned by GetReviewDiff when a branch diff fails.
type BranchDiffError struct {
	Branch string
	Err    error
}

func (e *BranchDiffError) Error() string {
	return fmt.Sprintf("could not get diff for branch %q: %v", e.Branch, e.Err)
}

func (e *BranchDiffError) Unwrap() error { return e.Err }

// ReviewDiff holds the result of GetReviewDiff.
type ReviewDiff struct {
	Diff          string
	Stat          string
	Commits       []Commit
	ContextHeader string
}

// GetReviewDiff fetches the diff, stat, commits and context header for a review.
func GetReviewDiff(ctx context.Context, params ReviewDiffParams) (ReviewDiff, error) {
	var r ReviewDiff

	switch {
	case params.pr != "":
		var err error
		r.Diff, err = GetPRDiff(ctx, params.pr)
		if err != nil {
			return r, err
		}
		r.Diff = truncateDiff(r.Diff, params.maxLines)
		if meta, metaErr := GetPRMeta(ctx, params.pr); metaErr == nil {
			if !params.withBody {
				meta.Body = ""
			}
			r.ContextHeader = FormatPRHeader(meta)
			r.Commits = meta.Commits
		}

	case params.branch != "":
		var err error
		r.Diff, err = GetBranchDiff(ctx, params.branch, params.maxLines)
		if err != nil {
			return r, &BranchDiffError{Branch: params.branch, Err: err}
		}
		r.Stat = GetBranchDiffStat(ctx, params.branch)
		r.Commits, _ = GetBranchCommits(ctx, params.branch)

	case params.commitHash != "":
		var err error
		r.Diff, err = GetDiff(ctx, params.commitHash)
		if err != nil {
			return r, err
		}
		r.Diff = truncateDiff(r.Diff, params.maxLines)
		r.Stat = GetCommitStat(ctx, params.commitHash)

	default:
		all := !params.stagedOnly
		var err error
		r.Diff, err = GetWorkingTreeDiff(ctx, all)
		if err != nil {
			return r, err
		}
		r.Diff = truncateDiff(r.Diff, params.maxLines)
		r.Stat = GetWorkingTreeStat(ctx, all)
		branch, _ := GetCurrentBranch(ctx)
		r.ContextHeader = FormatBranchHeader(branch)
	}

	return r, nil
}

func truncateDiff(diff string, maxLines uint32) string {
	if maxLines == 0 || diff == "" {
		return diff
	}
	lines := strings.SplitAfter(diff, "\n")
	if uint32(len(lines)) <= maxLines {
		return diff
	}
	return strings.Join(lines[:maxLines], "") + "... (truncated)\n"
}

// GetPRInfo returns a formatted string with commit messages and diff for a GitHub PR.
func GetPRInfo(ctx context.Context, prNumber string) (string, error) {
	meta, err := GetPRMeta(ctx, prNumber)
	if err != nil {
		return "", err
	}

	diff, err := GetPRDiff(ctx, prNumber)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "PR #%d: %s\n", meta.Number, meta.Title)
	fmt.Fprintf(&sb, "Total Commits: %d\n", len(meta.Commits))
	sb.WriteString("Commits:\n")
	for _, c := range meta.Commits {
		fmt.Fprintf(&sb, " - %s\n", c.Message)
		if c.Body != "" {
			fmt.Fprintf(&sb, "   %s\n", c.Body)
		}
	}
	sb.WriteString("\nDiffs:\n")
	sb.WriteString(diff)

	return sb.String(), nil
}
