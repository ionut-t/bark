package utils

import (
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/ionut-t/bark/internal/config"
)

// NormaliseCodeFences ensures code blocks and diff lines are not unnecessarily indented.
// It removes all leading whitespace from code fence markers and diff lines,
// while preserving internal indentation of the code/diff content.
func NormaliseCodeFences(content string) string {
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
	editor := config.GetEditor()

	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}
