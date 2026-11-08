package cmd

import (
	"fmt"
	"io"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")).Bold(true)

// printError writes err to w as a red "Error: <message>" line. w should be
// a colorprofile writer (fang passes one), which strips the styling when the
// output isn't a terminal or NO_COLOR is set, so scripts get plain text.
func printError(w io.Writer, err error) {
	_, _ = fmt.Fprintln(w, errorStyle.Render("Error: "+err.Error()))
}

// isPlainMode returns true if --plain is set or stdout is not a TTY.
func isPlainMode(cmd *cobra.Command) bool {
	plain, _ := cmd.Flags().GetBool("plain")
	return plain || !isatty.IsTerminal(os.Stdout.Fd())
}

// hasStdinData returns true if stdin has piped data (is not a TTY).
func hasStdinData() bool {
	return !isatty.IsTerminal(os.Stdin.Fd())
}

// readStdin reads all data from stdin and returns it as a string.
func readStdin() (string, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("error reading stdin: %w", err)
	}
	return string(data), nil
}

func readStdinIfPiped() (*string, error) {
	if hasStdinData() {
		stdinDiff, err := readStdin()
		if err != nil {
			return nil, err
		}

		if stdinDiff == "" {
			return nil, nil
		}

		return &stdinDiff, nil
	}

	return nil, nil
}
