package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

// stdinHintAfter is how long Bark waits for the first byte of a piped diff
// before saying on stderr what it is waiting for.
const stdinHintAfter = 3 * time.Second

const stdinHint = "bark: waiting for a diff on stdin. Pipe one in and close the pipe, or run Bark with </dev/null or a flag that names what to review, such as --branch or --staged, so it doesn't read stdin."

// readStdinIfPiped returns a diff piped on stdin, or nil. source says a
// flag names what to review (--branch, --pr, ...): stdin isn't read then,
// so a pipe an agent or task runner leaves open can't block the run.
// Otherwise Bark waits for the pipe to close, since a diff from a slow
// command (gh pr diff) may take a while to start, and says so on stderr
// if nothing has arrived after stdinHintAfter.
func readStdinIfPiped(source bool) (*string, error) {
	if source || !hasStdinData() {
		return nil, nil
	}
	diff, err := readPiped(os.Stdin, os.Stderr, stdinHintAfter)
	if err != nil || diff == "" {
		return nil, err
	}
	return &diff, nil
}

// readPiped reads all of r, writing stdinHint to hint if r has sent
// nothing after hintAfter.
func readPiped(r io.Reader, hint io.Writer, hintAfter time.Duration) (string, error) {
	fr := &firstRead{r: r}
	timer := time.AfterFunc(hintAfter, func() {
		if !fr.started.Load() {
			_, _ = fmt.Fprintln(hint, stdinHint)
		}
	})
	data, err := io.ReadAll(fr)
	timer.Stop()
	if err != nil {
		return "", fmt.Errorf("error reading stdin: %w", err)
	}
	return string(data), nil
}

// firstRead records whether a read from r has returned anything.
type firstRead struct {
	r       io.Reader
	started atomic.Bool
}

func (f *firstRead) Read(p []byte) (int, error) {
	n, err := f.r.Read(p)
	if n > 0 || err != nil {
		f.started.Store(true)
	}
	return n, err
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

// addTimeoutFlag adds --timeout, which overrides the llm_timeout config for
// one run.
func addTimeoutFlag(cmd *cobra.Command) {
	cmd.Flags().String("timeout", "", "How long one model call may take before Bark gives up, such as 10m or 90s (overrides config; default: 5m for reviews and their check, 3m for commit messages and PR descriptions in plain mode)")
}

// overrideTimeout applies --timeout to cfg when it was passed.
func overrideTimeout(cmd *cobra.Command, cfg config.Config) error {
	if !cmd.Flags().Changed("timeout") {
		return nil
	}
	timeout, _ := cmd.Flags().GetString("timeout")
	return cfg.OverrideLLMTimeout(timeout)
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
