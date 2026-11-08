// Package check runs a second model call over the findings of a structured
// review and marks the ones that don't hold up against the code as doubted.
// The first pass only sees the diff; the check also sees the full new
// version of every file a finding points to, and must quote the code a kept
// finding rests on. Doubted findings are set aside, not hidden, in case the
// check is wrong.
package check

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ionut-t/bark/v2/internal/debuglog"
	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/ionut-t/bark/v2/internal/llm"
)

//go:embed check.md
var system string

// System returns the system prompt for the checking call.
func System() string { return system }

// Result is a review after the check.
type Result struct {
	// Events are the review's events, with Doubt set on every finding the
	// check didn't keep. A kept finding may have a new severity, or have
	// lost a suggestion the check found broken.
	Events []findings.Event
	// Checked is the number of findings the check gave a verdict on, and
	// Doubted how many of those it didn't keep.
	Checked, Doubted int
}

// Run checks the findings in events with one call to client. Without
// findings, it makes no call. If the call fails, the error is returned and
// events should be used as they are.
//
// in.Files should hold every file the findings point to that can be read;
// LoadFiles fills it.
//
// startLog, if not nil, is called with the prompts before the call and
// returns the BARK_DEBUG log to write the output to (or nil).
func Run(ctx context.Context, client llm.LLM, in Input, events []findings.Event, startLog func(system, prompt string) *debuglog.Log) (Result, error) {
	fs := findingsIn(events)
	if len(fs) == 0 {
		return Result{Events: events}, nil
	}

	prompt := Prompt(in, fs)
	var log *debuglog.Log
	if startLog != nil {
		log = startLog(System(), prompt)
	}
	resp, err := client.Generate(ctx, System(), prompt)
	log.Write(resp.Content, resp.Usage)
	log.Finish(err)
	if err != nil {
		return Result{}, fmt.Errorf("error checking findings: %w", err)
	}

	outcomes := Apply(in, fs, ParseVerdicts(resp.Content))
	res := Result{Events: make([]findings.Event, len(events))}
	i := 0
	for j, e := range events {
		if e.Finding != nil {
			o := outcomes[i]
			i++
			f := o.Finding
			if !o.Unchecked {
				res.Checked++
			}
			if !o.Kept {
				res.Doubted++
				f.Doubt = o.Reason
				if f.Doubt == "" {
					f.Doubt = "the check gave no reason"
				}
			}
			e.Finding = &f
		}
		res.Events[j] = e
	}
	return res, nil
}

// LoadFiles reads every file the findings in events point to with read.
// Files that can't be read (deleted, binary or out of reach) are left out.
func LoadFiles(events []findings.Event, read func(path string) ([]byte, error)) map[string]string {
	fs := findingsIn(events)
	files := map[string]string{}
	for _, path := range referencedFiles(fs) {
		content, err := read(path)
		if err != nil || strings.IndexByte(string(content), 0) >= 0 {
			continue
		}
		files[path] = string(content)
	}
	return files
}

// Input is what the check needs besides the findings.
type Input struct {
	// Header is context about the change, such as the Go version line.
	Header string
	// Diff is the reviewed diff with new-side line numbers, as the first
	// pass saw it.
	Diff string
	// Files maps a path to its full new-side content. Files that couldn't be
	// read are left out; their findings are checked against the diff alone.
	Files map[string]string
}

// Prompt builds the user prompt for checking fs.
func Prompt(in Input, fs []findings.Finding) string {
	var b strings.Builder
	if h := strings.TrimSpace(in.Header); h != "" {
		b.WriteString(h)
		b.WriteString("\n\n")
	}
	b.WriteString("## Diff\n\n")
	b.WriteString(in.Diff)
	if !strings.HasSuffix(in.Diff, "\n") {
		b.WriteString("\n")
	}

	paths := make([]string, 0, len(in.Files))
	for _, f := range referencedFiles(fs) {
		if _, ok := in.Files[f]; ok {
			paths = append(paths, f)
		}
	}
	if len(paths) > 0 {
		b.WriteString("\n## Files\n\nThe full new version of each file the findings point to, with line numbers.\n")
		for _, p := range paths {
			fmt.Fprintf(&b, "\n### %s\n\n", p)
			writeNumbered(&b, in.Files[p])
		}
	}

	b.WriteString("\n## Findings\n\n")
	for i, f := range fs {
		fmt.Fprintf(&b, "%d. %s\n", i+1, findingJSON(f))
	}
	return b.String()
}

func referencedFiles(fs []findings.Finding) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range fs {
		if f.File != "" && !seen[f.File] {
			seen[f.File] = true
			out = append(out, f.File)
		}
	}
	sort.Strings(out)
	return out
}

func writeNumbered(b *strings.Builder, content string) {
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	for i, l := range lines {
		fmt.Fprintf(b, "%d\t%s\n", i+1, l)
	}
}

// findingJSON shows the model the neutral fields only; the persona's remark
// adds tone, not substance.
func findingJSON(f findings.Finding) string {
	v := struct {
		File       string `json:"file"`
		Line       int    `json:"line,omitempty"`
		EndLine    int    `json:"end_line,omitempty"`
		Severity   string `json:"severity"`
		Category   string `json:"category,omitempty"`
		Title      string `json:"title"`
		Body       string `json:"body"`
		Suggestion string `json:"suggestion,omitempty"`
	}{f.File, f.Line, f.EndLine, string(f.Severity), f.Category, f.Title, f.Body, f.Suggestion}
	out, _ := json.Marshal(v)
	return string(out)
}

// Verdict is the check's decision on one finding.
type Verdict struct {
	ID       int    `json:"id"`
	Verdict  string `json:"verdict"`
	Severity string `json:"severity"`
	// Suggestion is "ok" or "broken" for a kept finding with a suggestion.
	Suggestion string `json:"suggestion"`
	Evidence   string `json:"evidence"`
	Reason     string `json:"reason"`
}

// ParseVerdicts reads the check's JSON Lines output. Lines that aren't
// verdicts are ignored; a later verdict for the same id replaces an earlier
// one.
func ParseVerdicts(output string) map[int]Verdict {
	out := map[int]Verdict{}
	for raw := range strings.SplitSeq(output, "\n") {
		s := strings.TrimSpace(raw)
		if !strings.HasPrefix(s, "{") {
			continue
		}
		var v Verdict
		if json.Unmarshal([]byte(s), &v) != nil || v.ID <= 0 {
			continue
		}
		v.Verdict = strings.ToLower(strings.TrimSpace(v.Verdict))
		v.Severity = strings.ToLower(strings.TrimSpace(v.Severity))
		v.Suggestion = strings.ToLower(strings.TrimSpace(v.Suggestion))
		out[v.ID] = v
	}
	return out
}

// Outcome is what happened to one finding.
type Outcome struct {
	Finding findings.Finding
	Kept    bool
	// Unchecked is true when the check gave no verdict for the finding; it
	// is kept, so a check that fails part way never hides a finding.
	Unchecked bool
	// Reason says why the finding was kept or dropped.
	Reason string
}

// Apply decides each finding from the verdicts. A kept finding must come
// with evidence that appears in the diff or the files; otherwise it is
// dropped. A kept finding takes the check's severity, and loses its
// suggestion if the check found it broken.
func Apply(in Input, fs []findings.Finding, verdicts map[int]Verdict) []Outcome {
	code := newCorpus(in)
	out := make([]Outcome, len(fs))
	for i, f := range fs {
		v, ok := verdicts[i+1]
		switch {
		case !ok:
			out[i] = Outcome{Finding: f, Kept: true, Unchecked: true, Reason: "not checked"}
		case v.Verdict != "keep":
			out[i] = Outcome{Finding: f, Reason: v.Reason}
		case !code.contains(v.Evidence):
			out[i] = Outcome{Finding: f, Reason: "the check couldn't quote code that backs it"}
		default:
			if s := findings.Severity(v.Severity); isScale(s) {
				f.Severity = s
			}
			if v.Suggestion == "broken" {
				f.Suggestion, f.Original = "", ""
			}
			out[i] = Outcome{Finding: f, Kept: true, Reason: v.Reason}
		}
	}
	return out
}

func isScale(s findings.Severity) bool {
	switch s {
	case findings.Critical, findings.High, findings.Medium, findings.Low:
		return true
	}
	return false
}

// corpus is the code a verdict's evidence is looked up in: every line of the
// diff and the files, without line numbers, diff markers or surrounding
// whitespace.
type corpus struct {
	lines []string
}

func newCorpus(in Input) corpus {
	var c corpus
	for l := range strings.SplitSeq(in.Diff, "\n") {
		l = stripLineNumber(l)
		c.add(l)
		// Without the diff marker too; the marker can't be told apart from
		// code that starts with "-" or "+" after the whitespace is gone.
		if l != "" && strings.ContainsAny(l[:1], "+-") {
			c.add(l[1:])
		}
	}
	for _, content := range in.Files {
		for l := range strings.SplitSeq(content, "\n") {
			c.add(l)
		}
	}
	return c
}

func (c *corpus) add(l string) {
	if l = normalise(l); l != "" {
		c.lines = append(c.lines, l)
	}
}

// minEvidence is the shortest quoted line that counts as evidence; a lone
// "}" or ")" appears in every file and backs nothing.
const minEvidence = 3

// contains reports whether every line of evidence is part of some line of
// code, and at least one line is long enough to count. Models often trim a
// quoted line or quote only part of it, so a substring is enough.
func (c corpus) contains(evidence string) bool {
	found := false
	for l := range strings.SplitSeq(evidence, "\n") {
		l = normalise(stripEvidenceLine(l))
		if l == "" || l == "..." || l == "…" {
			continue
		}
		if len(l) < minEvidence {
			continue
		}
		if !c.has(l) {
			return false
		}
		found = true
	}
	return found
}

func (c corpus) has(s string) bool {
	for _, l := range c.lines {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

// stripLineNumber removes the line number NumberDiffLines puts before an
// added or context line, and the blank padding before a removed one.
func stripLineNumber(l string) string {
	t := strings.TrimLeft(l, " ")
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	if i > 0 && i < len(t) && t[i] == ' ' {
		return t[i+1:]
	}
	return t
}

// stripEvidenceLine removes a line number or diff marker the model copied
// along with the code despite being asked not to.
func stripEvidenceLine(l string) string {
	t := strings.TrimLeft(l, " ")
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	if i > 0 && i < len(t) && strings.ContainsRune("\t:| ", rune(t[i])) {
		t = t[i+1:]
	} else {
		t = l
	}
	t = strings.TrimLeft(t, " \t")
	if len(t) > 1 && (t[0] == '+' || t[0] == '-') && (t[1] == ' ' || t[1] == '\t') {
		t = t[1:]
	}
	return t
}

// normalise collapses runs of whitespace so indentation and spacing
// differences don't matter.
func normalise(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Review is a finished structured review to check.
type Review struct {
	Client     llm.LLM
	ReviewDiff git.ReviewDiff
	// Diff is the diff as the review saw it, with line numbers.
	Diff string
	// LogDir, Provider and Model are for the BARK_DEBUG log. OnLog, if not
	// nil, is told where the log is, or why it couldn't be started.
	LogDir, Provider, Model string
	OnLog                   func(path string, err error)
}

// Run reads the files the findings in events point to and checks them (see
// the package function Run).
func (r Review) Run(ctx context.Context, events []findings.Event) (Result, error) {
	files := LoadFiles(events, func(path string) ([]byte, error) {
		return r.ReviewDiff.NewSideFile(ctx, path)
	})
	in := Input{Header: r.ReviewDiff.ContextHeader, Diff: r.Diff, Files: files}
	return Run(ctx, r.Client, in, events, func(system, prompt string) *debuglog.Log {
		log, err := debuglog.Start(r.LogDir, "check", debuglog.Meta{
			Fields: []debuglog.Field{
				{Key: "provider", Value: r.Provider},
				{Key: "model", Value: r.Model},
			},
			System: system,
			Prompt: prompt,
		})
		if r.OnLog != nil && (err != nil || log != nil) {
			r.OnLog(log.Path(), err)
		}
		return log
	})
}

// Count returns the number of findings in events.
func Count(events []findings.Event) int {
	return len(findingsIn(events))
}

func findingsIn(events []findings.Event) []findings.Finding {
	var fs []findings.Finding
	for _, e := range events {
		if e.Finding != nil {
			fs = append(fs, *e.Finding)
		}
	}
	return fs
}
