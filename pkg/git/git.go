package git

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

var (
	ErrNotAGitRepository     = errors.New("not a git repository")
	ErrNoChangesInRepository = errors.New("no changes in repository")
	ErrNoCommitsInRepository = errors.New("no commits in repository")
)

// Commit represents a single git commit.
type Commit struct {
	Hash    string
	Author  string
	Date    string
	Message string
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
