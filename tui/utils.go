package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	editor "github.com/ionut-t/goeditor"
)

var viewMargin = lipgloss.NewStyle().Margin(2)

func writeToDisk(editor *editor.Model, filePath *string, content string) tea.Cmd {
	const duration = 3 * time.Second

	if filePath == nil || strings.TrimSpace(*filePath) == "" {
		return editor.DispatchError(errors.New("invalid file path"), duration)
	}

	if err := os.MkdirAll(filepath.Dir(*filePath), 0o755); err != nil {
		return editor.DispatchError(err, duration)
	}

	if err := os.WriteFile(*filePath, []byte(content), 0o644); err != nil {
		return editor.DispatchError(err, duration)
	}

	return editor.DispatchMessage("saved to "+*filePath, duration)
}

func DispatchNoSearchResultsError(editor *editor.Model) tea.Cmd {
	return editor.DispatchError(errors.New("no search results found"), 2*time.Second)
}

func detectProjectType() string {
	for _, c := range []struct {
		file    string
		project string
	}{
		{"go.mod", "Go"},
		{"Cargo.toml", "Rust"},
		{"build.zig", "Zig"},
		{"angular.json", "Angular"},
	} {
		if _, err := os.Stat(c.file); err == nil {
			return c.project
		}
	}

	if _, err := os.Stat("nx.json"); err == nil {
		if content, err := os.ReadFile("package.json"); err == nil {
			if strings.Contains(string(content), `"@angular/core"`) {
				return "Angular"
			}
		}
	}

	for _, f := range []string{"pyproject.toml", "requirements.txt", "setup.py"} {
		if _, err := os.Stat(f); err == nil {
			return "Python"
		}
	}

	return ""
}
