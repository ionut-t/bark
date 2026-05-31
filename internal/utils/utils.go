package utils

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/config"
)

var codeFenceEndPattern = regexp.MustCompile("(?m)([^\\n])(\\+```|-```)([^\\n\\s]*)\\s*$")

// NormaliseCodeFences ensures code blocks and diff lines are not unnecessarily indented.
// It removes all leading whitespace from code fence markers and diff lines,
// while preserving internal indentation of the code/diff content.
func NormaliseCodeFences(content string) string {
	// First pass: split lines that end with +``` or -``` onto separate lines
	content = codeFenceEndPattern.ReplaceAllString(content, "$1\n$2$3")

	lines := strings.Split(content, "\n")
	inCodeBlock := false

	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")

		// Always normalize code fence markers (remove all leading whitespace)
		if strings.HasPrefix(trimmed, "```") {
			lines[i] = trimmed
			inCodeBlock = !inCodeBlock
			continue
		}

		// Inside code blocks, handle diff markers specially
		if inCodeBlock {
			// For diff lines, remove leading whitespace before +/- but preserve everything after
			if strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "-") {
				// Get the marker symbol
				symbol := trimmed[0:1]
				// Everything after the symbol (may include tabs/spaces for indentation)
				afterSymbol := trimmed[1:]
				// Reconstruct: symbol at start + preserve rest exactly as-is
				lines[i] = symbol + afterSymbol
			}
			// For non-diff lines inside code blocks, leave unchanged to preserve formatting
		}
	}

	return strings.Join(lines, "\n")
}

// RemoveCodeFences removes markdown code fences from a given message string.
func RemoveCodeFences(message string) string {
	if message == "" {
		return ""
	}

	codeBlockRegex := regexp.MustCompile("(?s)```[a-z]*\n?(.*?)```")
	message = codeBlockRegex.ReplaceAllString(message, "$1")

	return strings.TrimSpace(message)
}

// OpenEditor opens the specified file in the user's preferred text editor.
func OpenEditor(path string) error {
	cmd, err := OpenInEditorCmd(config.GetEditor(), path)
	if err != nil {
		return err
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

// OpenInEditorCmd returns an exec.Cmd configured to open the specified file in the user's preferred text editor.
func OpenInEditorCmd(editor, path string) (*exec.Cmd, error) {
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return nil, fmt.Errorf("editor is not configured")
	}

	cmd := exec.Command(parts[0], append(parts[1:], path)...)

	return cmd, nil
}

// ReadLocalOverride reads the content of a .bark/ override file.
// Returns ("", nil) if the file does not exist or is empty — use the fallback.
// Returns ("", err) for any other I/O error — the caller should surface this to the user.
func ReadLocalOverride(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("could not read %s: %w", path, err)
	}
	if len(content) == 0 {
		return "", nil
	}
	return string(content), nil
}

// GetInstructions returns the content of a .bark/ override file, or fallback if the file does not exist or is empty.
func GetInstructions(path, fallback string) (string, error) {
	override, err := ReadLocalOverride(path)
	if err != nil {
		return "", err
	}

	if override != "" {
		return override, nil
	}

	return fallback, nil
}

func DispatchMsg(msg tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return msg
	}
}

type ClearMsg struct{}

func DispatchClearMsg(duration time.Duration) tea.Cmd {
	return tea.Tick(duration, func(t time.Time) tea.Msg {
		return ClearMsg{}
	})
}
