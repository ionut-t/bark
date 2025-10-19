package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ionut-t/bark/pkg/git"
	editor "github.com/ionut-t/goeditor/adapter-bubbletea"
)

func formatBranchInfo(branch *git.BranchInfo) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Branch: %s\n", branch.Name))
	sb.WriteString(fmt.Sprintf("Base Branch: %s\n", branch.BaseBranch))
	sb.WriteString(fmt.Sprintf("Total Commits: %d\n", len(branch.Commits)))
	sb.WriteString(fmt.Sprintf("Total Files Changed: %d\n", branch.TotalFilesChanged))
	sb.WriteString(fmt.Sprintf("Total Additions: %d\n", branch.TotalAdditions))
	sb.WriteString(fmt.Sprintf("Total Deletions: %d\n", branch.TotalDeletions))
	sb.WriteString("Commits:\n")

	for _, commit := range branch.Commits {
		sb.WriteString(fmt.Sprintf(" - %s\n", commit.Message))
		if commit.Body != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", commit.Body))
		}
	}

	sb.WriteString("\nDiffs:\n")
	sb.WriteString(branch.Diffs)

	return sb.String()
}

func writeToDisk(editor *editor.Model, filePath *string, content string) tea.Cmd {
	const duration = 3 * time.Second

	if filePath == nil || strings.TrimSpace(*filePath) == "" {
		return editor.DispatchError(errors.New("invalid file path"), duration)
	}

	if err := os.MkdirAll(filepath.Dir(*filePath), 0755); err != nil {
		return editor.DispatchError(err, duration)
	}

	if err := os.WriteFile(*filePath, []byte(content), 0644); err != nil {
		return editor.DispatchError(err, duration)
	}

	return editor.DispatchMessage("saved to "+*filePath, duration)
}
