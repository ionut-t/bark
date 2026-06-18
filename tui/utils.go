package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ionut-t/bark/v2/internal/config"
	editor "github.com/ionut-t/goeditor"
	"github.com/ionut-t/goeditor/core"
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

func createEditorStatusLine(args ...string) func(ctx editor.StatusLineContext) string {
	return func(ctx editor.StatusLineContext) string {
		statusLine := getEditorMode(ctx)
		cursorInfo := getEditorCursorInfo(ctx)

		var info strings.Builder

		for _, arg := range args {
			info.WriteString(" | ")
			info.WriteString(arg)
		}

		infoStr := info.String()
		width := ctx.Width - (lipgloss.Width(cursorInfo) + lipgloss.Width(statusLine) + lipgloss.Width(infoStr))
		gap := strings.Repeat(" ", max(0, width))

		statusLine += ctx.Theme.StatusLineStyle.Render(
			gap + cursorInfo + infoStr,
		)

		return statusLine
	}
}

func getEditorCursorInfo(ctx editor.StatusLineContext) string {
	return fmt.Sprintf("%d:%d", ctx.Cursor.Position.Row+1, ctx.Cursor.Position.Col+1)
}

func getEditorMode(ctx editor.StatusLineContext) string {
	switch ctx.State.Mode {
	case core.NormalMode:
		return ctx.Theme.NormalModeStyle.Render(" NORMAL ")
	case core.InsertMode:
		return ctx.Theme.InsertModeStyle.Render(" INSERT ")
	case core.VisualMode:
		return ctx.Theme.VisualModeStyle.Render(" VISUAL ")
	case core.VisualLineMode:
		return ctx.Theme.VisualModeStyle.Render(" VISUAL LINE ")
	case core.CommandMode:
		return ctx.Theme.CommandModeStyle.Render(" COMMAND ")
	case core.SearchMode:
		return ctx.Theme.SearchModeStyle.Render(" SEARCH ")
	default:
		return ""
	}
}
