package git

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
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

// GetCommits returns a list of the most recent commits.
func GetCommits(limit int) ([]Commit, error) {
	cmd := exec.Command("git", "log", "--pretty=format:%H|%an|%ar|%s", "-n", strconv.Itoa(limit))

	output, err := cmd.Output()

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
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
func GetCommit(hash string) (*Commit, error) {
	cmd := exec.Command("git", "show", "--pretty=format:%H|%an|%ar|%s", "-s", hash)
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
func GetDiff(hash string) (string, error) {
	if !IsGitRepo() {
		return "", ErrNotAGitRepository
	}

	cmd := exec.Command("git", "show", hash)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get diff for commit %s: %w", hash, err)
	}

	return string(output), nil
}

// GetWorkingTreeDiff returns the current uncommitted changes in the working directory.
func GetWorkingTreeDiff(all bool) (string, error) {
	if !IsGitRepo() {
		return "", ErrNotAGitRepository
	}

	var cmd *exec.Cmd
	if all {
		cmd = exec.Command("git", "diff", "HEAD")
	} else {
		cmd = exec.Command("git", "diff", "--staged")
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

func GetCurrentBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

func GetBranchDiff(branch string) (string, error) {
	cmd := exec.Command("git", "diff", branch)
	output, err := cmd.Output()

	if err != nil {
		return "", fmt.Errorf("failed to get branch diff: %w", err)
	}

	// truncate diff if too large
	// for now, just limit the number of lines
	maxLines := 2000
	lines := strings.Split(string(output), "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines = append(lines, "... (truncated)")
		output = []byte(strings.Join(lines, "\n"))
	}

	return string(output), nil
}

func CommitChanges(message string, all bool) error {
	if all {
		cmd := exec.Command("git", "add", "-A")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to stage changes: %w\n\n %s", err, string(out))
		}
	}

	cmd := exec.Command("git", "commit", "-m", message)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to commit changes: %w\n\n %s", err, string(out))
	}

	return nil
}

// GetBaseBranch attempts to determine the base branch (main, master, develop)
func GetBaseBranch() (string, error) {
	// Try to find the default branch from remote
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
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
func GetBranchCommits(baseBranch string) ([]Commit, error) {
	currentBranch, err := GetCurrentBranch()
	if err != nil {
		return nil, err
	}

	// Get commits that are in current branch but not in base
	// Format: %H = hash, %an = author, %ar = date relative, %s = subject, %b = body
	cmd := exec.Command("git", "log",
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
func GetBranchStats(baseBranch string) (filesChanged, additions, deletions int, err error) {
	currentBranch, err := GetCurrentBranch()
	if err != nil {
		return 0, 0, 0, err
	}

	cmd := exec.Command("git", "diff", "--shortstat",
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
func GetBranchInfo(baseBranch string) (*BranchInfo, error) {
	currentBranch, err := GetCurrentBranch()
	if err != nil {
		return nil, err
	}

	if baseBranch == "" {
		baseBranch, err = GetBaseBranch()

		if err != nil {
			return nil, err
		}
	}

	commits, err := GetBranchCommits(baseBranch)
	if err != nil {
		return nil, err
	}

	if len(commits) == 0 {
		return nil, fmt.Errorf("no commits found on branch %s", currentBranch)
	}

	diffs, err := GetBranchDiff(baseBranch)
	if err != nil {
		return nil, err
	}

	filesChanged, additions, deletions, err := GetBranchStats(baseBranch)

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
