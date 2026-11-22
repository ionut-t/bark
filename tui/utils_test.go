package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ionut-t/bark/v2/internal/config"
	editor "github.com/ionut-t/goeditor"
	"github.com/ionut-t/goeditor/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var keyEsc = tea.KeyPressMsg{Code: tea.KeyEscape}

func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	require.NotNil(t, cmd)
	return cmd()
}

func TestPromptPreview(t *testing.T) {
	assert.Equal(t, "SYS\n\n---\n\nPROMPT\n\n", promptPreview("SYS", "PROMPT"))
}

func TestWriteToDisk(t *testing.T) {
	ed := newEditor(80, 20)

	path := filepath.Join(t.TempDir(), "nested", "out.md")
	assert.NotNil(t, writeToDisk(&ed, &path, "content"))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "content", string(got))

	blank := "  "
	assert.NotNil(t, writeToDisk(&ed, &blank, "x"))
	assert.NotNil(t, writeToDisk(&ed, nil, "x"))

	// A file where a directory must go can't be written.
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o644))
	inside := filepath.Join(file, "out.md")
	assert.NotNil(t, writeToDisk(&ed, &inside, "x"))
	_, err = os.Stat(inside)
	assert.Error(t, err)

	assert.NotNil(t, dispatchNoSearchResultsError(&ed))
}

func TestSetRelativeNumberCmd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := config.New(path, config.Data{})

	assert.Nil(t, setRelativeNumberCmd(cfg, true)())
	assert.True(t, cfg.GetRelativeNumber())

	bad := config.New(filepath.Join(t.TempDir(), "missing", "dir", "config.toml"), config.Data{})
	_, isErr := setRelativeNumberCmd(bad, true)().(configErrMsg)
	assert.True(t, isErr, "a config that can't be saved is reported")
}

// keyPress is a key typed as text, such as 'i' or '/'.
func keyPress(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func TestEditorStatusLineHelpers(t *testing.T) {
	theme := editor.Theme{
		NormalModeStyle:  lipgloss.NewStyle(),
		InsertModeStyle:  lipgloss.NewStyle(),
		VisualModeStyle:  lipgloss.NewStyle(),
		CommandModeStyle: lipgloss.NewStyle(),
		SearchModeStyle:  lipgloss.NewStyle(),
	}
	ctx := func(mode core.Mode) editor.StatusLineContext {
		return editor.StatusLineContext{State: core.State{Mode: mode}, Theme: theme}
	}

	assert.Equal(t, " NORMAL ", getEditorMode(ctx(core.NormalMode)))
	assert.Equal(t, " INSERT ", getEditorMode(ctx(core.InsertMode)))
	assert.Equal(t, " VISUAL ", getEditorMode(ctx(core.VisualMode)))
	assert.Equal(t, " VISUAL LINE ", getEditorMode(ctx(core.VisualLineMode)))
	assert.Equal(t, " COMMAND ", getEditorMode(ctx(core.CommandMode)))
	assert.Equal(t, " SEARCH ", getEditorMode(ctx(core.SearchMode)))
	assert.Empty(t, getEditorMode(ctx(core.Mode("other"))))

	c := ctx(core.NormalMode)
	c.Cursor.Position = core.Position{Row: 4, Col: 9}
	assert.Equal(t, "5:10", getEditorCursorInfo(c), "shown one-based")

	c.Width = 40
	line := createEditorStatusLine("gemini")(c)
	assert.True(t, strings.HasPrefix(line, " NORMAL "))
	assert.True(t, strings.HasSuffix(line, "5:10 | gemini"))
	assert.Equal(t, 40, lipgloss.Width(line))
}
