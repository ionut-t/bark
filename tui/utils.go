package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	editor "github.com/ionut-t/goeditor/adapter-bubbletea"
)

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

func DispatchNoSearchResultsError(editor *editor.Model) tea.Cmd {
	return editor.DispatchError(errors.New("no search results found"), 2*time.Second)
}
