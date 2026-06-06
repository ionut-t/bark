package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ionut-t/bark/v2/internal/config"
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

func dispatchNoSearchResultsError(editor *editor.Model) tea.Cmd {
	return editor.DispatchError(errors.New("no search results found"), 2*time.Second)
}

type configErrMsg error

func setRelativeNumberCmd(cfg config.Config, value bool) tea.Cmd {
	return func() tea.Msg {
		if err := cfg.SetRelativeNumber(value); err != nil {
			return configErrMsg(err)
		}

		return nil
	}
}
