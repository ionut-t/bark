// Package debuglog writes a debug log of each model call when BARK_DEBUG is
// set: the prompts, the raw model output (before any rendering), timings and
// token usage, one file per call in <storage>/logs.
package debuglog

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ionut-t/bark/v2/internal/llm"
)

// EnvVar turns logging on when set to a true value ("1", "true", ...).
const EnvVar = "BARK_DEBUG"

// Field is one line of a log's header.
type Field struct {
	Key   string
	Value string
}

// Meta describes the call being logged: header fields (provider, model and
// whatever the caller finds useful) and the prompts sent.
type Meta struct {
	Fields []Field
	System string
	Prompt string
}

// Log is one call's log file. A nil *Log is valid and does nothing, so
// callers don't need to check whether logging is on.
type Log struct {
	file    *os.File
	start   time.Time
	first   time.Duration
	usage   *llm.Usage
	written bool
}

// Enabled reports whether BARK_DEBUG turns logging on.
func Enabled() bool {
	on, _ := strconv.ParseBool(os.Getenv(EnvVar))
	return on
}

// Start creates the log file for a call under dir/logs and writes the header
// and prompts. kind names the command (e.g. "review") and goes into the file
// name. Call Start right before the model call: timings are measured from
// here. It returns nil, nil when logging is off.
func Start(dir, kind string, meta Meta) (*Log, error) {
	if !Enabled() {
		return nil, nil
	}
	return start(dir, kind, meta, time.Now())
}

func start(dir, kind string, meta Meta, now time.Time) (*Log, error) {
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating log directory: %w", err)
	}

	path := filepath.Join(logDir, now.Format("2006-01-02T15-04-05.000")+"-"+kind+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("creating log file: %w", err)
	}

	l := &Log{file: file, start: now}
	l.header(append([]Field{{"kind", kind}, {"time", now.Format(time.RFC3339)}}, meta.Fields...))
	l.section("system")
	l.printf("%s\n", meta.System)
	l.section("prompt")
	l.printf("%s\n", meta.Prompt)
	l.section("response (raw)")

	return l, nil
}

// Path is the log file's path.
func (l *Log) Path() string {
	if l == nil {
		return ""
	}
	return l.file.Name()
}

// Write appends a chunk of raw model output. The file is written as chunks
// arrive, so a call cut short still leaves what arrived.
func (l *Log) Write(content string, usage *llm.Usage) {
	if l == nil {
		return
	}
	if content != "" && !l.written {
		l.first = time.Since(l.start)
		l.written = true
	}
	if usage != nil {
		u := *usage
		l.usage = &u
	}
	l.printf("%s", content)
}

// Finish writes timings, token usage and err (if any), and closes the file.
func (l *Log) Finish(err error) {
	if l == nil {
		return
	}
	total := time.Since(l.start)

	l.printf("\n")
	l.section("stats")
	if l.written {
		l.printf("first output: %.1fs\n", l.first.Seconds())
	} else {
		l.printf("first output: none\n")
	}
	l.printf("total:        %.1fs\n", total.Seconds())
	if u := l.usage; u != nil {
		l.printf("tokens:       %d input, %d output, %d total\n", u.InputTokens, u.OutputTokens, u.TotalTokens)
		// Gemini counts thinking in the total but not in the output.
		if other := u.TotalTokens - u.InputTokens - u.OutputTokens; other > 0 {
			l.printf("              %d other (thinking)\n", other)
		}
	}
	if err != nil {
		l.printf("error:        %v\n", err)
	}

	_ = l.file.Close()
}

// header writes fields one per line, with the values lined up.
func (l *Log) header(fields []Field) {
	width := 0
	for _, f := range fields {
		width = max(width, len(f.Key)+1)
	}
	for _, f := range fields {
		l.printf("%-*s %s\n", width, f.Key+":", f.Value)
	}
}

func (l *Log) section(name string) {
	l.printf("\n===== %s =====\n", name)
}

// printf writes to the log, ignoring errors: a debug log must never fail
// the call it logs.
func (l *Log) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(l.file, format, args...)
}
