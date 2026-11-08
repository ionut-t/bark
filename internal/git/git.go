package git

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
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

// commitFormat prints a commit's hash, author, relative date, subject and body
// separated by NUL bytes. Use it with -z so commits are separated by NUL too:
// every commit is then exactly commitFieldCount fields. Unlike "|", a NUL can't
// appear in an author name or commit message.
const (
	commitFormat     = "--pretty=format:%H%x00%an%x00%ar%x00%s%x00%b"
	commitFieldCount = 5
)

// parseCommits parses git output produced with -z and commitFormat.
func parseCommits(output []byte) []Commit {
	if len(output) == 0 {
		return nil
	}

	fields := strings.Split(string(output), "\x00")
	commits := make([]Commit, 0, len(fields)/commitFieldCount)
	for f := range slices.Chunk(fields, commitFieldCount) {
		if len(f) < commitFieldCount {
			break
		}
		commits = append(commits, Commit{
			Hash:    f[0],
			Author:  f[1],
			Date:    f[2],
			Message: f[3],
			Body:    strings.TrimSpace(f[4]),
		})
	}

	return commits
}

// GetCommits returns a list of the most recent commits.
func GetCommits(ctx context.Context, limit int) ([]Commit, error) {
	cmd := exec.CommandContext(ctx, "git", "log", "-z", commitFormat, "-n", strconv.Itoa(limit))

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

	return parseCommits(output), nil
}

// GetCommit returns a single commit.
func GetCommit(ctx context.Context, hash string) (*Commit, error) {
	cmd := exec.CommandContext(ctx, "git", "show", "-s", "-z", commitFormat, hash)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get commit %s: %w", hash, err)
	}

	commits := parseCommits(output)
	if len(commits) == 0 {
		return nil, nil
	}

	return &commits[0], nil
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

	if all {
		cleanup, err := withUntrackedIndex(ctx, cmd)
		if err != nil {
			return "", fmt.Errorf("failed to get working tree diff: %w", err)
		}
		defer cleanup()
	}

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
	cmd := exec.CommandContext(ctx, "git", args...)
	if all {
		cleanup, err := withUntrackedIndex(ctx, cmd)
		if err != nil {
			return ""
		}
		defer cleanup()
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// withUntrackedIndex points cmd at a temporary copy of the index in which every untracked,
// non-ignored file is marked intent-to-add, so "git diff HEAD" includes new files as
// additions. The user's real index is never modified. The returned cleanup removes the
// temporary index and must be called after cmd has run.
func withUntrackedIndex(ctx context.Context, cmd *exec.Cmd) (cleanup func(), err error) {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--git-path", "index").Output()
	if err != nil {
		return nil, fmt.Errorf("could not locate git index: %w", err)
	}
	indexPath := strings.TrimSpace(string(out))

	tmpDir, err := os.MkdirTemp("", "bark-index-*")
	if err != nil {
		return nil, fmt.Errorf("could not create temporary index: %w", err)
	}

	cleanup = func() { _ = os.RemoveAll(tmpDir) }

	tmpIndex := filepath.Join(tmpDir, "index")

	if err := copyIndex(indexPath, tmpIndex); err != nil {
		cleanup()
		return nil, err
	}

	// Build on cmd's own environment so anything the caller set on it is kept.
	env := append(cmd.Environ(), "GIT_INDEX_FILE="+tmpIndex)
	// ":/" is the repository root, so new files outside the current directory are included.
	add := exec.CommandContext(ctx, "git", "add", "--intent-to-add", "--", ":/")
	add.Env = env
	if out, err := add.CombinedOutput(); err != nil {
		cleanup()
		return nil, fmt.Errorf("could not mark untracked files: %w: %s", err, strings.TrimSpace(string(out)))
	}

	cmd.Env = env
	return cleanup, nil
}

// copyIndex copies the index at src to dst. When there is no index yet (nothing has been
// staged), dst is not created and git starts from an empty index; a zero-byte file would
// be rejected as corrupt.
func copyIndex(src, dst string) error {
	in, err := os.Open(src)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("could not read git index: %w", err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("could not write temporary index: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("could not write temporary index: %w", err)
	}
	// Close can report a failed write, so its error must not be dropped.
	if err := out.Close(); err != nil {
		return fmt.Errorf("could not write temporary index: %w", err)
	}
	return nil
}

// branchRange returns the "<branch>...HEAD" range, which diffs HEAD against its merge base
// with branch so commits added to branch after the fork are not shown as reversed changes.
func branchRange(branch string) string {
	return branch + "...HEAD"
}

// GetBranchDiffStat returns the --stat summary for the changes on the current branch since it
// forked from branch.
func GetBranchDiffStat(ctx context.Context, branch string) string {
	args := append([]string{"diff", "--stat", branchRange(branch), "--"}, defaultDiffExcludes...)
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

// GetBranchDiff returns the changes on the current branch since it forked from branch.
func GetBranchDiff(ctx context.Context, branch string, maxLines uint32) (string, error) {
	args := append([]string{"diff", branchRange(branch), "--"}, defaultDiffExcludes...)
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
	// Trim the full "refs/remotes/origin/" prefix so a default branch with a
	// slash (e.g. release/2.x) keeps its full name. --short isn't used: it
	// prints "remotes/origin/..." when a local branch shadows the short name.
	cmd := exec.CommandContext(ctx, "git", "symbolic-ref", "refs/remotes/origin/HEAD")
	output, err := cmd.Output()
	if err == nil {
		if branch, ok := strings.CutPrefix(strings.TrimSpace(string(output)), "refs/remotes/origin/"); ok && branch != "" {
			return branch, nil
		}
	}

	// origin/HEAD is only set by git clone; repos created with git init +
	// remote add don't have it, so fall back to the common base branch names.
	// Names are tried in order; for each name the local branch wins over the
	// remote-tracking one, so a local develop doesn't beat origin/main.
	for _, name := range []string{"main", "master", "develop"} {
		for _, candidate := range []struct{ ref, base string }{
			{"refs/heads/" + name, name},
			{"refs/remotes/origin/" + name, "origin/" + name},
		} {
			if exec.CommandContext(ctx, "git", "rev-parse", "--verify", "--quiet", candidate.ref).Run() == nil {
				return candidate.base, nil
			}
		}
	}

	return "", fmt.Errorf("could not determine base branch: origin/HEAD is not set and no main, master or develop branch exists; pass it with --branch")
}

// GetBranchCommits gets all commits on current branch that aren't in base branch
func GetBranchCommits(ctx context.Context, baseBranch string) ([]Commit, error) {
	currentBranch, err := GetCurrentBranch(ctx)
	if err != nil {
		return nil, err
	}

	// Get commits that are in current branch but not in base
	cmd := exec.CommandContext(ctx, "git", "log", "-z", commitFormat,
		fmt.Sprintf("%s..%s", baseBranch, currentBranch))

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get branch commits: %w", err)
	}

	commits := parseCommits(output)
	if len(commits) == 0 {
		return nil, fmt.Errorf("no commits found on branch %s", currentBranch)
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
	// HeadSHA is the commit the pull request's branch points to.
	HeadSHA string
}

// GetPRMeta fetches the title and commit messages for a GitHub pull request via the gh CLI.
func GetPRMeta(ctx context.Context, prNumber string) (*PRMeta, error) {
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", prNumber, "--json", "commits,title,number,body,headRefOid")
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
		HeadSHA string `json:"headRefOid"`
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

	return &PRMeta{Number: raw.Number, Title: raw.Title, Body: raw.Body, Commits: commits, HeadSHA: raw.HeadSHA}, nil
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
	// Ref is the git revision whose file content matches the diff's new-side
	// line numbers: a commit hash, ":" for the index, or "" for the working tree.
	Ref string
	// SkipEnrichment is true when no local ref matches the diff (e.g. a PR head
	// that is not checked out), so enclosing-context extraction must be skipped.
	SkipEnrichment bool

	// source is where NewSideFile reads files from: a revision, or a pull
	// request's head (prHead). A ReviewDiff not made by GetReviewDiff has
	// no source.
	source string
	prHead string
}

// sourceWorkingTree is the source of a working-tree diff; "" means none.
const sourceWorkingTree = "worktree"

// NewSideFile returns path as it is on the new side of the diff, so its line
// numbers match the diff's. For a pull request it reads the head commit,
// from the local repository if it has it, otherwise from GitHub.
func (r ReviewDiff) NewSideFile(ctx context.Context, path string) ([]byte, error) {
	switch {
	case r.prHead != "":
		if out, err := GetFileContent(ctx, r.prHead, path); err == nil {
			return out, nil
		}
		return ghFileContent(ctx, r.prHead, path)
	case r.source == sourceWorkingTree:
		return GetFileContent(ctx, "", path)
	case r.source != "":
		return GetFileContent(ctx, r.source, path)
	}
	return nil, errors.New("no source for the new side of the diff")
}

// ghFileContent reads path at commit sha from the current repository on
// GitHub.
func ghFileContent(ctx context.Context, sha, path string) ([]byte, error) {
	endpoint := "repos/{owner}/{repo}/contents/" + (&url.URL{Path: path}).EscapedPath() + "?ref=" + url.QueryEscape(sha)
	cmd := exec.CommandContext(ctx, "gh", "api", "-H", "Accept: application/vnd.github.raw", endpoint)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read %s at %s from GitHub: %w", path, sha, err)
	}
	return out, nil
}

// GetReviewDiff fetches the diff, stat, commits and context header for a review.
func GetReviewDiff(ctx context.Context, params ReviewDiffParams) (ReviewDiff, error) {
	var r ReviewDiff

	switch {
	case params.pr != "":
		// The PR head is generally not checked out locally, so no local ref
		// matches the diff's line numbers.
		r.SkipEnrichment = true
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
			r.prHead = meta.HeadSHA
		}

	case params.branch != "":
		var err error
		r.Diff, err = GetBranchDiff(ctx, params.branch, params.maxLines)
		if err != nil {
			return r, &BranchDiffError{Branch: params.branch, Err: err}
		}
		r.Stat = GetBranchDiffStat(ctx, params.branch)
		// The diff is <branch>...HEAD, so uncommitted changes aren't in it.
		r.source = "HEAD"
		r.Commits, _ = GetBranchCommits(ctx, params.branch)

	case params.commitHash != "":
		r.Ref = params.commitHash
		r.source = params.commitHash
		var err error
		r.Diff, err = GetDiff(ctx, params.commitHash)
		if err != nil {
			return r, err
		}
		r.Diff = truncateDiff(r.Diff, params.maxLines)
		r.Stat = GetCommitStat(ctx, params.commitHash)

	default:
		all := !params.stagedOnly
		r.source = sourceWorkingTree
		if params.stagedOnly {
			r.Ref = ":"
			r.source = ":"
		}
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

	r.ContextHeader += goVersionHeader(ctx, r.Diff)

	return r, nil
}

// goVersionHeader reads go.mod at the repository root. For a pull request,
// that's the checked-out go.mod, which is the PR's own in CI.
func goVersionHeader(ctx context.Context, diff string) string {
	root, err := RepoRoot(ctx)
	if err != nil {
		return ""
	}
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	return FormatGoVersionHeader(goMod, diff)
}

func truncateDiff(diff string, maxLines uint32) string {
	if maxLines == 0 || diff == "" {
		return diff
	}

	var count uint32
	for i := 0; i < len(diff); i++ {
		if diff[i] == '\n' {
			count++
			if count >= maxLines {
				return diff[:i+1] + "... (truncated)\n"
			}
		}
	}
	return diff
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

var (
	repoRootMu   sync.Mutex
	repoRootPath string
)

// RepoRoot returns the absolute path of the repository's top-level directory.
// Only a successful result is memoized, so a transient failure (e.g. a
// cancelled ctx on the first call) doesn't poison later calls.
func RepoRoot(ctx context.Context) (string, error) {
	repoRootMu.Lock()
	defer repoRootMu.Unlock()
	if repoRootPath != "" {
		return repoRootPath, nil
	}
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("could not resolve repository root: %w", err)
	}
	repoRootPath = strings.TrimSpace(string(output))
	return repoRootPath, nil
}

// GetFileContent returns the content of a file at a specific git ref or index.
func GetFileContent(ctx context.Context, ref string, filePath string) ([]byte, error) {
	if !IsGitRepo() {
		return nil, ErrNotAGitRepository
	}

	// If ref is empty, we read the working-tree file. Diff paths are always
	// repo-root-relative, so anchor them there rather than at the process cwd.
	if ref == "" {
		root, err := RepoRoot(ctx)
		if err != nil {
			return nil, err
		}
		return os.ReadFile(filepath.Join(root, filePath))
	}

	// Build the git revision spec. A bare ":" means the staged/index version (":path"),
	// otherwise it's "ref:path".
	var arg string
	if ref == ":" {
		arg = ":" + filePath
	} else {
		arg = fmt.Sprintf("%s:%s", ref, filePath)
	}
	cmd := exec.CommandContext(ctx, "git", "show", arg)
	return cmd.Output()
}
