package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/tui"
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

// warnRepoLLM tells the user when the repository's config picks the
// provider or model for this run instead of their own config: running Bark
// in a cloned repository shouldn't silently switch which model, and which of
// their keys, it uses. Call it after applying --provider and --model.
func warnRepoLLM(w io.Writer, cfg config.Config) {
	if msg := cfg.RepoLLMWarning(); msg != "" {
		_, _ = fmt.Fprintln(w, "bark: warning: "+msg)
	}
}

const plainFlag = "plain"

// addPlainFlag declares --plain on cmd. Only commands that check
// isPlainMode get it, so the others don't accept a flag they'd ignore.
func addPlainFlag(cmd *cobra.Command) {
	cmd.Flags().Bool(plainFlag, false, "Output plain text instead of TUI (auto-detected when stdout is piped)")
}

// isPlainMode returns true if --plain is set or stdout is not a TTY.
func isPlainMode(cmd *cobra.Command) bool {
	plain, _ := cmd.Flags().GetBool(plainFlag)
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

// addEffortFlag adds --effort, which overrides the effort config for one run.
func addEffortFlag(cmd *cobra.Command) {
	cmd.Flags().String("effort", "", "How hard the model thinks before answering: "+strings.Join(config.Efforts(), ", ")+"; lower is faster and cheaper, and Ollama ignores it (overrides config; default: the model's own)")
}

// overrideEffort applies --effort to cfg when it was passed.
func overrideEffort(cmd *cobra.Command, cfg config.Config) error {
	if !cmd.Flags().Changed("effort") {
		return nil
	}
	effort, _ := cmd.Flags().GetString("effort")
	return cfg.OverrideEffort(effort)
}

// runTUI runs the TUI with opts. Warnings about secrets masked before a
// call (see redact.Wrap) are held until the TUI exits, so they don't break
// its screen, then written to stderr.
func runTUI(opts tui.Options) error {
	var warnings syncBuffer
	opts.Warnings = &warnings
	_, err := tea.NewProgram(tui.New(opts)).Run()
	_, _ = os.Stderr.Write(warnings.Bytes())
	if err != nil {
		return fmt.Errorf("error running UI: %w", err)
	}
	return nil
}

// syncBuffer is a bytes.Buffer safe to write from the TUI's goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Bytes()
}
