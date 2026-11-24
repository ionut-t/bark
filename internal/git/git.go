package git

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/ionut-t/bark/v2/internal/github"
)

var (
	ErrNotAGitRepository     = errors.New("not a git repository")
	ErrNoChangesInRepository = errors.New("no changes in repository")
	ErrNoCommitsInRepository = errors.New("no commits in repository")
)

var shortStatRegex = regexp.MustCompile(`(?P<files>\d+) files? changed(?:, (?P<additions>\d+) insertions?\(\+\))?(?:, (?P<deletions>\d+) deletions?\(-\))?`)

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

	args := append([]string{"show", hash, "--"}, excludes(ctx)...)
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
	cmd := exec.CommandContext(ctx, "git", append(baseArgs, excludes(ctx)...)...)

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

// workingTreePaths lists every file with uncommitted changes, ignored or
// not: staged ones only unless all, with untracked ones if all, as
// GetWorkingTreeDiff diffs them.
func workingTreePaths(ctx context.Context, all bool) []string {
	if !all {
		return changedPaths(exec.CommandContext(ctx, "git", "diff", "--name-only", "-z", "--staged"))
	}
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", "-z", "HEAD")
	cleanup, err := withUntrackedIndex(ctx, cmd)
	if err != nil {
		return nil
	}
	defer cleanup()
	return changedPaths(cmd)
}

// GetWorkingTreeStat returns the --stat summary for uncommitted working tree changes.
// Errors are treated as best-effort: a non-nil error returns an empty string.
func GetWorkingTreeStat(ctx context.Context, all bool) string {
	var args []string
	if all {
		args = append([]string{"diff", "--stat", "HEAD", "--"}, excludes(ctx)...)
	} else {
		args = append([]string{"diff", "--stat", "--staged", "--"}, excludes(ctx)...)
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
	return rangeStat(ctx, branchRange(branch))
}

// rangeStat returns the --stat summary for the diff of rng, a range such
// as "<base>...<head>", or "" if git can't make it.
func rangeStat(ctx context.Context, rng string) string {
	args := append([]string{"diff", "--stat", rng, "--"}, excludes(ctx)...)
	out, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// GetCommitStat returns the --stat summary for a single commit.
func GetCommitStat(ctx context.Context, hash string) string {
	args := append([]string{"show", "--stat", hash, "--"}, excludes(ctx)...)
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

// GetBranchDiff returns the changes on the current branch since it forked
// from branch, cut to limit.
func GetBranchDiff(ctx context.Context, branch string, limit DiffLimit) (string, error) {
	diff, _, err := branchDiff(ctx, branch, limit)
	return diff, err
}

// branchDiff is GetBranchDiff that also returns the cut, or nil if the whole
// diff fits.
func branchDiff(ctx context.Context, branch string, limit DiffLimit) (string, *DiffCut, error) {
	return rangeDiff(ctx, branchRange(branch), limit)
}

// rangeDiff is branchDiff for any range, such as "<base>...<head>". The
// whole diff is read before it is cut, so its files can be ranked.
func rangeDiff(ctx context.Context, rng string, limit DiffLimit) (string, *DiffCut, error) {
	args := append([]string{"diff", rng, "--"}, excludes(ctx)...)
	output, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return "", nil, fmt.Errorf("failed to get branch diff: %w", err)
	}
	diff, cut := truncateDiff(string(output), limit)
	return diff, cut, nil
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

// GetBranchStats gets addition/deletion stats for the branch compared to
// base, leaving out ignored files as GetBranchDiff does.
func GetBranchStats(ctx context.Context, baseBranch string) (filesChanged, additions, deletions int, err error) {
	currentBranch, err := GetCurrentBranch(ctx)
	if err != nil {
		return 0, 0, 0, err
	}

	// The same files as GetBranchDiff, so the totals describe what the
	// model sees.
	args := append([]string{"diff", "--shortstat", fmt.Sprintf("%s...%s", baseBranch, currentBranch), "--"}, excludes(ctx)...)
	cmd := exec.CommandContext(ctx, "git", args...)
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
func GetBranchInfo(ctx context.Context, baseBranch string, limit DiffLimit) (*BranchInfo, error) {
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

	diffs, err := GetBranchDiff(ctx, baseBranch, limit)
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

// GetPRDiff returns the diff for a GitHub pull request.
func GetPRDiff(ctx context.Context, prNumber string) (string, error) {
	diff, _, err := prDiff(ctx, prNumber)
	return diff, err
}

// prDiff is GetPRDiff that also returns the paths it left out.
func prDiff(ctx context.Context, prNumber string) (diff string, ignored []string, err error) {
	raw, err := github.New().PullRequestDiff(ctx, prNumber)
	if err != nil {
		return "", nil, err
	}
	diff, ignored = filterDiff(raw, loadIgnore(ctx))
	return diff, ignored, nil
}

var diffFileHeaderRe = regexp.MustCompile(`^diff --git a/.+ b/(.+)$`)

// filterDiff removes the file sections of a unified git diff whose path the
// rules ignore, and returns what's left and the paths it removed.
func filterDiff(diff string, rules ignoreRules) (string, []string) {
	lines := strings.Split(diff, "\n")
	out := make([]string, 0, len(lines))
	var ignored []string
	skip := false

	for _, line := range lines {
		if m := diffFileHeaderRe.FindStringSubmatch(line); m != nil {
			skip = rules.matches(m[1])
			if skip {
				ignored = append(ignored, m[1])
			}
		}
		if !skip {
			out = append(out, line)
		}
	}

	return strings.Join(out, "\n"), ignored
}

// changedPaths runs cmd, a git command that lists paths with --name-only
// -z, and returns them, or nil if it fails. It reads every path the change
// touches, ignored or not.
func changedPaths(cmd *exec.Cmd) []string {
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var paths []string
	for p := range strings.SplitSeq(string(out), "\x00") {
		if p = strings.TrimSpace(p); p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// PRMeta holds lightweight metadata about a GitHub pull request.
type PRMeta struct {
	Number  int
	Title   string
	Body    string
	Commits []Commit
	// HeadSHA is the commit the pull request's branch points to.
	HeadSHA string
	// BaseSHA is the commit the branch the pull request merges into
	// points to.
	BaseSHA string
	// Files are the files the pull request changes, as GitHub lists
	// them: it may list fewer than ChangedFiles.
	Files        []PRFile
	ChangedFiles int
}

// PRFile is a file a pull request changes.
type PRFile = github.File

// GetPRMeta fetches the title and commit messages for a GitHub pull request.
func GetPRMeta(ctx context.Context, prNumber string) (*PRMeta, error) {
	raw, err := github.New().PullRequest(ctx, prNumber)
	if err != nil {
		return nil, err
	}

	commits := make([]Commit, len(raw.Commits))
	for i, c := range raw.Commits {
		commits[i] = Commit{Message: c.Headline, Body: c.Body}
	}

	return &PRMeta{
		Number:       raw.Number,
		Title:        raw.Title,
		Body:         raw.Body,
		Commits:      commits,
		HeadSHA:      raw.HeadSHA,
		BaseSHA:      raw.BaseSHA,
		Files:        raw.Files,
		ChangedFiles: raw.ChangedFiles,
	}, nil
}

// formatPRFileStat formats the files GitHub lists for a pull request like
// git diff --stat, leaving out the files rules ignore. It returns "" if
// GitHub listed none.
func formatPRFileStat(meta *PRMeta, rules ignoreRules) string {
	var files []PRFile
	width, most := 0, 0
	for _, f := range meta.Files {
		if rules.matches(f.Path) {
			continue
		}
		files = append(files, f)
		width = max(width, len(f.Path))
		most = max(most, f.Additions+f.Deletions)
	}
	if len(files) == 0 {
		return ""
	}

	countWidth := len(strconv.Itoa(most))
	var sb strings.Builder
	added, deleted := 0, 0
	for _, f := range files {
		added += f.Additions
		deleted += f.Deletions
		plus, minus := statBar(f.Additions, f.Deletions, most)
		fmt.Fprintf(&sb, " %-*s | %*d %s%s\n", width, f.Path, countWidth, f.Additions+f.Deletions,
			strings.Repeat("+", plus), strings.Repeat("-", minus))
	}

	fmt.Fprintf(&sb, " %d %s changed", len(files), plural(len(files), "file", "files"))
	if added > 0 {
		fmt.Fprintf(&sb, ", %d %s(+)", added, plural(added, "insertion", "insertions"))
	}
	if deleted > 0 {
		fmt.Fprintf(&sb, ", %d %s(-)", deleted, plural(deleted, "deletion", "deletions"))
	}
	if len(meta.Files) < meta.ChangedFiles {
		fmt.Fprintf(&sb, "\n (GitHub listed only %d of the %d changed files)", len(meta.Files), meta.ChangedFiles)
	}
	return sb.String()
}

// statGraphWidth is the width of git diff --stat's graph on a wide terminal.
const statGraphWidth = 50

// statBar returns how many + and - git diff --stat draws for a file with
// add additions and del deletions when the file with the most changes has
// most. Like git (diff.c), it scales the file's total first and then
// splits it, and draws at least one of each sign the file has, so a small
// mixed file can get a longer bar than a slightly bigger one-sided file.
func statBar(add, del, most int) (plus, minus int) {
	if most <= statGraphWidth {
		return add, del
	}
	scale := func(n int) int {
		if n == 0 {
			return 0
		}
		return 1 + n*(statGraphWidth-1)/most
	}
	total := scale(add + del)
	if total < 2 && add > 0 && del > 0 {
		total = 2
	}
	if add < del {
		plus = scale(add)
		return plus, total - plus
	}
	minus = scale(del)
	return total - minus, minus
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// hasCommit reports whether the local repository has the commit sha.
func hasCommit(ctx context.Context, sha string) bool {
	return sha != "" && exec.CommandContext(ctx, "git", "cat-file", "-e", sha+"^{commit}").Run() == nil
}

// ReviewDiffParams controls which diff GetReviewDiff fetches.
type ReviewDiffParams struct {
	pr         string
	branch     string
	limit      DiffLimit
	commitHash string
	stagedOnly bool
	withBody   bool
	since      string
	sinceBase  string
}

func PRDiff(prNumber string) ReviewDiffParams {
	return ReviewDiffParams{pr: prNumber}
}

func BranchDiff(branch string) ReviewDiffParams {
	return ReviewDiffParams{branch: branch}
}

// WithLimit returns a copy of the params with limit applied to the diff.
// Every source is cut the same way, after the whole diff is fetched.
func (p ReviewDiffParams) WithLimit(limit DiffLimit) ReviewDiffParams {
	p.limit = limit
	return p
}

// WithPRDescription includes the PR body in the review context.
// Only has effect when used with PRDiff.
func (p ReviewDiffParams) WithPRDescription() ReviewDiffParams {
	p.withBody = true
	return p
}

// Since returns a copy of the params that, for a pull request, diff only
// from sha, the commit an earlier review was made at, to the pull
// request's head. base is the pull request's merge base then, if known.
// When that can't be done from the local repository (sha or the head
// isn't there, sha is no longer in the pull request's history, the branch
// merged other commits since, or the merge base changed), the whole pull
// request is diffed and ReviewDiff.FullReason says why. Only has effect
// with PRDiff.
func (p ReviewDiffParams) Since(sha, base string) ReviewDiffParams {
	p.since, p.sinceBase = sha, base
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
	// Cut says how much of the diff was kept if it was cut to fit its
	// limit; nil if the whole diff is there.
	Cut *DiffCut
	// Warnings say what the review is missing or where its diff came
	// from when that isn't the usual place, e.g. a pull request whose
	// metadata couldn't be read.
	Warnings []string
	// Since is the commit the diff starts at when it holds only what
	// changed in a pull request since an earlier review
	// (ReviewDiffParams.Since), and "" when it's the whole pull request.
	Since string
	// FullReason says why the whole pull request was diffed although
	// ReviewDiffParams.Since asked for less.
	FullReason string
	// Ignored are the files the change touches, all of which .barkignore
	// (or the defaults) leaves out, when that is why Diff is empty. It is
	// nil when Diff isn't empty, when nothing changed, and for a diff
	// since an earlier review (Since).
	Ignored []string

	// source is where NewSideFile reads files from: a revision, or a pull
	// request's head (prHead). A ReviewDiff not made by GetReviewDiff has
	// no source.
	source string
	prHead string
	// prMergeBase is where the pull request's head branched off its base,
	// if both commits are local.
	prMergeBase string
	// prDiff is the whole pull request's diff when Diff holds only what
	// changed since an earlier review.
	prDiff string
}

// IgnoredOnly says that r's diff is empty because every file the change
// touches is one Bark ignores (Ignored), naming the first few. change
// names the change with its verb, e.g. "commit abc1234 changes".
func (r ReviewDiff) IgnoredOnly(change string) string {
	const shown = 3
	files := strings.Join(r.Ignored[:min(shown, len(r.Ignored))], ", ")
	if len(r.Ignored) > shown {
		files += fmt.Sprintf(" and %d more", len(r.Ignored)-shown)
	}
	return fmt.Sprintf("%s only files Bark ignores (%s), so there's nothing to review", change, files)
}

// PRHead returns the pull request's head commit the diff was made at, or ""
// if it isn't a pull request's diff or its metadata couldn't be read.
func (r ReviewDiff) PRHead() string {
	return r.prHead
}

// PRMergeBase returns the commit the pull request's head branched off its
// base at, or "" if it isn't a pull request's diff or the commits aren't
// in the local repository. It changes when the pull request is retargeted
// or rebased, not when its base branch moves on.
func (r ReviewDiff) PRMergeBase() string {
	return r.prMergeBase
}

// PRChangedPaths returns every path the pull request changes from where it
// branched off its base to its head, both sides of a rename, including the
// ones .barkignore leaves out of the diff. It needs the pull request's
// commits in the local repository: GitHub's list of a pull request's files
// can be cut short and names only the new side of a rename.
func (r ReviewDiff) PRChangedPaths(ctx context.Context) ([]string, error) {
	if r.prHead == "" || r.prMergeBase == "" {
		return nil, errors.New("the pull request's commits aren't in the local repository (check out with fetch-depth: 0)")
	}
	out, err := exec.CommandContext(ctx, "git", "diff", "--name-only", "-z", "--no-renames", "--no-ext-diff",
		r.prMergeBase, r.prHead, "--").Output()
	if err != nil {
		return nil, fmt.Errorf("could not list the pull request's files: %w", err)
	}
	var paths []string
	for p := range strings.SplitSeq(string(out), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// CommentDiff returns the diff whose lines GitHub accepts review comments
// on: the whole pull request's, which is more than Diff when Diff holds
// only what changed since an earlier review.
func (r ReviewDiff) CommentDiff() string {
	if r.prDiff != "" {
		return r.prDiff
	}
	return r.Diff
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

// searchSource returns where Grep and RepoFile look: a revision, ":" for the
// index, or "" for the working tree. A pull request whose head isn't in
// the local repository is searched in the working tree, which in CI is
// the checked-out pull request.
func (r ReviewDiff) searchSource(ctx context.Context) (string, error) {
	switch {
	case r.prHead != "":
		if hasCommit(ctx, r.prHead) {
			return r.prHead, nil
		}
		return "", nil
	case r.source == sourceWorkingTree:
		return "", nil
	case r.source != "":
		return r.source, nil
	}
	return "", errors.New("no source for the new side of the diff")
}

// Grep returns the repository-relative paths of the files that contain word
// as a whole word, at the version of the repository the diff's new side
// belongs to, among the files matching the glob patterns (e.g. "*.go").
func (r ReviewDiff) Grep(ctx context.Context, word string, globs ...string) ([]string, error) {
	src, err := r.searchSource(ctx)
	if err != nil {
		return nil, err
	}
	root, err := RepoRoot(ctx)
	if err != nil {
		return nil, err
	}

	args := []string{"grep", "-l", "-w", "-F", "--full-name", "-e", word}
	switch src {
	case "":
		args = append(args, "--untracked")
	case ":":
		args = append(args, "--cached")
	default:
		args = append(args, src)
	}
	args = append(args, "--")
	args = append(args, globs...)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && exitErr.ExitCode() == 1 {
		return nil, nil // no matches
	}
	if err != nil {
		return nil, fmt.Errorf("git grep %q: %w", word, err)
	}

	var paths []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if src != "" && src != ":" {
			line = strings.TrimPrefix(line, src+":")
		}
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths, nil
}

// IgnoredFiles returns a check for the repository-relative paths
// .barkignore (with the defaults) leaves out of the diff. Related-code
// lookup uses it to prefer definitions outside them when a name is
// ambiguous.
func (r ReviewDiff) IgnoredFiles(ctx context.Context) func(path string) bool {
	return loadIgnore(ctx).matches
}

// GrepWords is Grep for many words in one git process: it maps each word to
// the files that contain it. Words found nowhere are left out.
func (r ReviewDiff) GrepWords(ctx context.Context, words []string, globs ...string) (map[string][]string, error) {
	src, err := r.searchSource(ctx)
	if err != nil {
		return nil, err
	}
	root, err := RepoRoot(ctx)
	if err != nil {
		return nil, err
	}
	if len(words) == 0 {
		return map[string][]string{}, nil
	}

	// -o prints each match as path:word; patterns come from stdin.
	args := []string{"grep", "-o", "-w", "-F", "--full-name", "-f", "-"}
	switch src {
	case "":
		args = append(args, "--untracked")
	case ":":
		args = append(args, "--cached")
	default:
		args = append(args, src)
	}
	args = append(args, "--")
	args = append(args, globs...)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(strings.Join(words, "\n") + "\n")
	out, err := cmd.Output()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && exitErr.ExitCode() == 1 {
		return map[string][]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("git grep: %w", err)
	}

	found := map[string][]string{}
	seen := map[string]bool{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if src != "" && src != ":" {
			line = strings.TrimPrefix(line, src+":")
		}
		i := strings.LastIndexByte(line, ':')
		if i <= 0 {
			continue
		}
		path, word := line[:i], line[i+1:]
		if key := word + "\x00" + path; !seen[key] {
			seen[key] = true
			found[word] = append(found[word], path)
		}
	}
	return found, nil
}

// RepoFile reads path from the version of the repository Grep searches.
// Unlike NewSideFile it never asks GitHub, so for a pull request whose
// head isn't local, a file the pull request changes may not match the
// diff's line numbers; use NewSideFile for those.
func (r ReviewDiff) RepoFile(ctx context.Context, path string) ([]byte, error) {
	src, err := r.searchSource(ctx)
	if err != nil {
		return nil, err
	}
	return GetFileContent(ctx, src, path)
}

// ghFileContent reads path at commit sha from the current repository on
// GitHub.
func ghFileContent(ctx context.Context, sha, path string) ([]byte, error) {
	out, err := github.New().FileContent(ctx, sha, path)
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
		if err := r.loadPR(ctx, params); err != nil {
			return r, err
		}

	case params.branch != "":
		var err error
		r.Diff, r.Cut, err = branchDiff(ctx, params.branch, params.limit)
		if err != nil {
			return r, &BranchDiffError{Branch: params.branch, Err: err}
		}
		r.Stat = GetBranchDiffStat(ctx, params.branch)
		// The diff is <branch>...HEAD, so uncommitted changes aren't in it.
		r.source = "HEAD"
		r.Commits, _ = GetBranchCommits(ctx, params.branch)
		if r.Diff == "" {
			r.Ignored = changedPaths(exec.CommandContext(ctx, "git", "diff", "--name-only", "-z", branchRange(params.branch)))
		}

	case params.commitHash != "":
		r.source = params.commitHash
		var err error
		r.Diff, err = GetDiff(ctx, params.commitHash)
		if err != nil {
			return r, err
		}
		r.Diff, r.Cut = truncateDiff(r.Diff, params.limit)
		r.Stat = GetCommitStat(ctx, params.commitHash)
		if r.Diff == "" {
			r.Ignored = changedPaths(exec.CommandContext(ctx, "git", "show", "--name-only", "-z", "--format=", params.commitHash))
		}

	default:
		all := !params.stagedOnly
		r.source = sourceWorkingTree
		if params.stagedOnly {
			r.source = ":"
		}
		var err error
		r.Diff, err = GetWorkingTreeDiff(ctx, all)
		if err != nil {
			return r, err
		}
		r.Diff, r.Cut = truncateDiff(r.Diff, params.limit)
		r.Stat = GetWorkingTreeStat(ctx, all)
		if r.Diff == "" {
			r.Ignored = workingTreePaths(ctx, all)
		}
		branch, _ := GetCurrentBranch(ctx)
		r.ContextHeader = FormatBranchHeader(branch)
	}

	r.ContextHeader += r.goVersionHeader(ctx)

	return r, nil
}

// loadPR fills r with the diff, stat, commits and context header of the
// pull request params.pr.
func (r *ReviewDiff) loadPR(ctx context.Context, params ReviewDiffParams) error {
	meta, err := GetPRMeta(ctx, params.pr)
	if err != nil {
		meta = nil
		r.Warnings = append(r.Warnings, fmt.Sprintf(
			"could not read pull request #%s's title, commits and head commit, so the review goes without them: %s",
			params.pr, oneLine(err)))
	} else {
		r.Commits = meta.Commits
		r.prHead = meta.HeadSHA
		withBody := *meta
		if !params.withBody {
			withBody.Body = ""
		}
		r.ContextHeader = FormatPRHeader(&withBody)
	}

	// The pull request's commits, if the local repository has them (CI
	// checks out with fetch-depth: 0). Three dots diff from the merge
	// base, as GitHub does.
	var local string
	if meta != nil && hasCommit(ctx, meta.BaseSHA) && hasCommit(ctx, meta.HeadSHA) {
		local = meta.BaseSHA + "..." + meta.HeadSHA
		r.prMergeBase = mergeBase(ctx, meta.BaseSHA, meta.HeadSHA)
	}

	if params.since != "" {
		r.FullReason = r.loadSince(ctx, params.since, params.sinceBase, meta, params.limit)
		if r.FullReason == "" {
			return nil
		}
	}

	diff, ignored, err := prDiff(ctx, params.pr)
	switch {
	case err == nil:
		r.Diff, r.Cut = truncateDiff(diff, params.limit)
		if r.Diff == "" {
			r.Ignored = ignored
		}
	case local != "":
		// GitHub refuses the diff of a very large pull request.
		var localErr error
		r.Diff, r.Cut, localErr = rangeDiff(ctx, local, params.limit)
		if localErr != nil {
			return fmt.Errorf("%w (the local diff of its commits failed too: %v)", err, localErr)
		}
		if r.Diff == "" {
			r.Ignored = changedPaths(exec.CommandContext(ctx, "git", "diff", "--name-only", "-z", local))
		}
		r.Warnings = append(r.Warnings, fmt.Sprintf(
			"GitHub didn't give pull request #%s's diff (%s), so Bark reviews the local diff of its commits instead",
			params.pr, oneLine(err)))
	default:
		return err
	}

	if local != "" {
		r.Stat = rangeStat(ctx, local)
	}
	if r.Stat == "" && meta != nil {
		r.Stat = formatPRFileStat(meta, loadIgnore(ctx))
	}
	return nil
}

// loadSince fills r with what changed in the pull request of meta since
// commit since, and returns "", or returns why it can't and leaves r as it
// was.
func (r *ReviewDiff) loadSince(ctx context.Context, since, base string, meta *PRMeta, limit DiffLimit) string {
	short := since[:min(7, len(since))]
	if meta == nil {
		return "could not read the pull request's head commit"
	}
	head := meta.HeadSHA
	if !hasCommit(ctx, head) {
		return fmt.Sprintf("the pull request's head commit %s isn't in the local repository (check out with fetch-depth: 0)", head[:min(7, len(head))])
	}
	if !hasCommit(ctx, since) {
		return fmt.Sprintf("%s, the commit the last review was made at, isn't in the local repository: it was rebased or force-pushed away, or the checkout is shallow", short)
	}
	if !isAncestor(ctx, since, head) {
		return fmt.Sprintf("%s is no longer in the pull request's history (rebase or force-push)", short)
	}
	// A retargeted pull request (say, a stacked one whose base was merged)
	// changes more than its commits show.
	if base != "" && r.prMergeBase == "" {
		return fmt.Sprintf("could not tell whether the pull request's base changed since %s: its base commit isn't in the local repository", short)
	}
	if base != "" && r.prMergeBase != base {
		return fmt.Sprintf("the pull request's base changed since %s (retargeted at another branch)", short)
	}
	rng := since + ".." + head
	// A merge brings in the changes of another branch, usually the base,
	// which aren't the pull request's.
	merges, err := exec.CommandContext(ctx, "git", "rev-list", "--merges", "-n", "1", rng).Output()
	if err != nil {
		return fmt.Sprintf("could not list the commits since %s: %s", short, oneLine(err))
	}
	if strings.TrimSpace(string(merges)) != "" {
		return fmt.Sprintf("the branch merged other commits since %s", short)
	}
	out, err := exec.CommandContext(ctx, "git", "log", "--reverse", "-z", commitFormat, rng).Output()
	if err != nil {
		return fmt.Sprintf("could not list the commits since %s: %s", short, oneLine(err))
	}
	diff, cut, err := rangeDiff(ctx, rng, limit)
	if err != nil {
		return fmt.Sprintf("could not diff the changes since %s: %s", short, oneLine(err))
	}

	if r.prMergeBase != "" {
		// Not cut: it only says which lines can take comments.
		if whole, _, err := rangeDiff(ctx, r.prMergeBase+".."+head, DiffLimit{}); err == nil {
			r.prDiff = whole
		}
	}

	r.Diff, r.Cut, r.Since = diff, cut, since
	r.Commits = parseCommits(out)
	r.Stat = rangeStat(ctx, rng)
	r.ContextHeader += fmt.Sprintf("### Scope\n\nThis diff holds only the changes since Bark's last review of this pull request, at %s; the rest of the pull request was reviewed then.\n\n", short)
	return ""
}

// mergeBase returns the best common ancestor of commits a and b, or "" if
// git can't find one.
func mergeBase(ctx context.Context, a, b string) string {
	out, err := exec.CommandContext(ctx, "git", "merge-base", a, b).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// isAncestor reports whether commit a is an ancestor of commit b, or b
// itself.
func isAncestor(ctx context.Context, a, b string) bool {
	return exec.CommandContext(ctx, "git", "merge-base", "--is-ancestor", a, b).Run() == nil
}

// oneLine is err's message on one line.
func oneLine(err error) string {
	return strings.Join(strings.Fields(err.Error()), " ")
}

// goVersionHeader reads go.mod as it is on the new side of the diff for a
// pull request, and otherwise, or if that fails, at the repository root.
func (r ReviewDiff) goVersionHeader(ctx context.Context) string {
	// Skip reading go.mod (maybe from GitHub) for a diff with no Go files.
	if !goFileInDiff.MatchString(r.Diff) {
		return ""
	}
	var goMod []byte
	if r.prHead != "" {
		if out, err := r.NewSideFile(ctx, "go.mod"); err == nil {
			goMod = out
		}
	}
	if goMod == nil {
		root, err := RepoRoot(ctx)
		if err != nil {
			return ""
		}
		if goMod, err = os.ReadFile(filepath.Join(root, "go.mod")); err != nil {
			return ""
		}
	}
	return FormatGoVersionHeader(goMod, r.Diff)
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
	repoRootMu sync.Mutex
	// repoRoots maps a working directory to its repository's root.
	repoRoots = map[string]string{}
)

// RepoRoot returns the absolute path of the repository's top-level directory.
// Only a successful result is memoized, per working directory, so a
// transient failure (e.g. a cancelled ctx on the first call) doesn't poison
// later calls and a change of directory finds the right repository.
func RepoRoot(ctx context.Context) (string, error) {
	cwd, _ := os.Getwd()
	repoRootMu.Lock()
	defer repoRootMu.Unlock()
	if root, ok := repoRoots[cwd]; ok && cwd != "" {
		return root, nil
	}
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("could not resolve repository root: %w", err)
	}
	root := strings.TrimSpace(string(output))
	repoRoots[cwd] = root
	return root, nil
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
