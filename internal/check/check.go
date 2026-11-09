// Package check runs a second model call over the findings of a structured
// review and marks the ones that don't hold up against the code as doubted.
// The first pass sees the diff; the check also sees the new version of
// every file a finding points to (whole, or the lines around each finding
// for a long file) and related code from other files, and must quote the
// code a kept finding rests on. Doubted findings are set aside, not hidden, in case the check is
// wrong.
package check

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/ionut-t/bark/v2/internal/debuglog"
	"github.com/ionut-t/bark/v2/internal/enclosing"
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
	// Checked is the number of findings the check gave a verdict on,
	// Doubted how many of those it doubted, and RaisedBefore how many it
	// found repeat a point raised before on the pull request (these have
	// RaisedBefore set instead of Doubt).
	Checked, Doubted, RaisedBefore int
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
			switch {
			case o.RaisedBefore != "":
				res.RaisedBefore++
				f.RaisedBefore = o.RaisedBefore
			case !o.Kept:
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
	// Long files are cut to the lines around the findings (see excerpts).
	Files map[string]string
	// Related is code from other files around the findings: definitions of
	// what the findings' functions use, and their callers (see
	// enclosing.Related).
	Related []enclosing.Piece
	// Prior is what earlier reviews of the same pull request raised, oldest
	// first. The check drops a finding that repeats one of these points
	// (see priorPoints for what it's shown).
	Prior []findings.Finding
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

	if ex := excerpts(in.Files, fs); len(ex) > 0 {
		b.WriteString("\n## Files\n\nThe new version of each file the findings point to, with line numbers: " +
			"the whole file, or for a long file the lines around each finding.\n")
		for _, e := range ex {
			e.write(&b)
		}
	}

	if rel := enclosing.FormatRelated(in.Related); rel != "" {
		b.WriteString("\n")
		b.WriteString(rel)
	}

	if points := priorPoints(in.Prior); len(points) > 0 {
		b.WriteString("\n## Raised before on this pull request\n\n" +
			"Earlier reviews of this pull request already reported these points. Line numbers are from earlier commits.\n\n")
		for i, p := range points {
			fmt.Fprintf(&b, "%d. `%s` %s: %s\n", i+1, location(p), oneLine(p.Title), findings.FirstSentence(p.Body))
		}
	}

	b.WriteString("\n## Findings\n\n")
	for i, f := range fs {
		fmt.Fprintf(&b, "%d. %s\n", i+1, findingJSON(f))
	}
	return b.String()
}

// maxPriorPoints caps the points raised before that the check is shown, so
// a pull request with many review rounds doesn't make the prompt long.
const maxPriorPoints = 40

// priorPoints returns the points raised before that the check is shown and
// numbered by: each point once (the same file and title, in any case, is
// one point; its most recent copy is kept), the most recent maxPriorPoints.
func priorPoints(prior []findings.Finding) []findings.Finding {
	seen := map[string]bool{}
	var out []findings.Finding
	for _, p := range slices.Backward(prior) {
		key := p.File + "\x00" + strings.ToLower(oneLine(p.Title))
		if p.File == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
		if len(out) == maxPriorPoints {
			break
		}
	}
	slices.Reverse(out)
	return out
}

// raisedBeforeRe matches the start of a drop reason that names the point
// raised before which the finding repeats (see check.md).
var raisedBeforeRe = regexp.MustCompile(`(?i)^\s*raised before\s*\((\d+)\)`)

// raisedBefore returns the point raised before that a drop reason names, if
// it's one of points and about the same file as f.
func raisedBefore(reason string, f findings.Finding, points []findings.Finding) (findings.Finding, bool) {
	m := raisedBeforeRe.FindStringSubmatch(reason)
	if m == nil {
		return findings.Finding{}, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 1 || n > len(points) || points[n-1].File != f.File {
		return findings.Finding{}, false
	}
	return points[n-1], true
}

// location is "file:line" or "file:line-end" for a point raised before.
func location(f findings.Finding) string {
	switch {
	case f.Line <= 0:
		return f.File
	case f.EndLine > f.Line:
		return fmt.Sprintf("%s:%d-%d", f.File, f.Line, f.EndLine)
	}
	return fmt.Sprintf("%s:%d", f.File, f.Line)
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

// Limits on how much of the files the check sends, so a long file doesn't
// make the call slow, expensive or too big for the model. About 14 tokens
// per line of Go.
const (
	// wholeFileLines is the longest file sent whole.
	wholeFileLines = 1500
	// windowLines is how many lines before and after a finding are sent
	// from a longer file.
	windowLines = 150
	// maxSentLines caps the lines sent from all files together. Files that
	// don't fit are left out and their findings are checked against the
	// diff alone.
	maxSentLines = 4000
)

// excerpt is the part of a file the check is shown: ranges of 1-based,
// inclusive line numbers.
type excerpt struct {
	path   string
	lines  []string
	ranges [][2]int
}

func (e excerpt) write(b *strings.Builder) {
	fmt.Fprintf(b, "\n### %s\n\n", e.path)
	whole := len(e.ranges) == 1 && e.ranges[0] == [2]int{1, len(e.lines)}
	if !whole {
		fmt.Fprintf(b, "(%d lines; only the lines around the findings are shown)\n", len(e.lines))
	}
	for i, r := range e.ranges {
		if i > 0 || (!whole && r[0] > 1) {
			b.WriteString("...\n")
		}
		for n := r[0]; n <= r[1]; n++ {
			fmt.Fprintf(b, "%d\t%s\n", n, e.lines[n-1])
		}
	}
	if last := e.ranges[len(e.ranges)-1]; last[1] < len(e.lines) {
		b.WriteString("...\n")
	}
}

// excerpts picks what to show of each file the findings point to: the whole
// file up to wholeFileLines, otherwise windowLines around each finding, all
// within maxSentLines. Files are taken in path order; one that doesn't fit
// in what's left of the budget is left out.
func excerpts(files map[string]string, fs []findings.Finding) []excerpt {
	var out []excerpt
	budget := maxSentLines
	for _, path := range referencedFiles(fs) {
		content, ok := files[path]
		if !ok {
			continue
		}
		lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
		ranges := [][2]int{{1, len(lines)}}
		if len(lines) > wholeFileLines {
			ranges = windows(fs, path, len(lines))
		}
		size := 0
		for _, r := range ranges {
			size += r[1] - r[0] + 1
		}
		if size > budget {
			continue
		}
		budget -= size
		out = append(out, excerpt{path: path, lines: lines, ranges: ranges})
	}
	return out
}

// windows returns the merged ranges within windowLines of the findings on
// path, in a file of n lines. Findings without a line get the top of the
// file.
func windows(fs []findings.Finding, path string, n int) [][2]int {
	var rs [][2]int
	for _, f := range fs {
		if f.File != path {
			continue
		}
		start, end := 1, 2*windowLines
		if f.Line > 0 {
			// A line past the end of the file (the model got it wrong, or
			// the file changed) gets the end of the file. Unclamped, start
			// would pass end and slicing the range would panic.
			first, last := min(f.Line, n), min(max(f.Line, f.EndLine), n)
			start, end = first-windowLines, last+windowLines
		}
		rs = append(rs, [2]int{max(start, 1), min(end, n)})
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i][0] < rs[j][0] })

	merged := rs[:1]
	for _, r := range rs[1:] {
		last := &merged[len(merged)-1]
		if r[0] <= last[1]+1 {
			last[1] = max(last[1], r[1])
			continue
		}
		merged = append(merged, r)
	}
	return merged
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
		if json.Unmarshal([]byte(s), &v) != nil && json.Unmarshal([]byte(escapeControl(s)), &v) != nil {
			continue
		}
		if v.ID <= 0 {
			continue
		}
		v.Verdict = strings.ToLower(strings.TrimSpace(v.Verdict))
		v.Severity = strings.ToLower(strings.TrimSpace(v.Severity))
		v.Suggestion = strings.ToLower(strings.TrimSpace(v.Suggestion))
		out[v.ID] = v
	}
	return out
}

// escapeControl escapes the control characters inside the strings of a
// JSON line. Models copy code with raw tabs into quoted evidence, which
// JSON doesn't allow.
func escapeControl(s string) string {
	var b strings.Builder
	inString, escaped := false, false
	for _, r := range s {
		switch {
		case escaped:
			escaped = false
		case inString && r == '\\':
			escaped = true
		case r == '"':
			inString = !inString
		case inString && r < 0x20:
			fmt.Fprintf(&b, "\\u%04x", r)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
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
	// RaisedBefore is set when the finding was dropped as a repeat of a
	// point raised before: that point's title and location.
	RaisedBefore string
}

// Apply decides each finding from the verdicts. A kept finding must come
// with evidence that appears in the diff or the files; otherwise it is
// dropped. A kept finding takes the check's severity, and loses its
// suggestion if the check found it broken.
func Apply(in Input, fs []findings.Finding, verdicts map[int]Verdict) []Outcome {
	code := newCorpus(in.Diff, excerpts(in.Files, fs), in.Related)
	points := priorPoints(in.Prior)
	out := make([]Outcome, len(fs))
	for i, f := range fs {
		v, ok := verdicts[i+1]
		switch {
		case !ok:
			out[i] = Outcome{Finding: f, Kept: true, Unchecked: true, Reason: "not checked"}
		case v.Verdict != "keep":
			out[i] = Outcome{Finding: f, Reason: v.Reason}
			if p, ok := raisedBefore(v.Reason, f, points); ok {
				out[i].RaisedBefore = fmt.Sprintf("%s (`%s`)", oneLine(p.Title), location(p))
			}
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
// diff and of the files as the check was shown them, without line numbers,
// diff markers or surrounding whitespace.
type corpus struct {
	lines []string
}

func newCorpus(diff string, files []excerpt, related []enclosing.Piece) corpus {
	var c corpus
	for l := range strings.SplitSeq(diff, "\n") {
		l = stripLineNumber(l)
		c.add(l)
		// Without the diff marker too; the marker can't be told apart from
		// code that starts with "-" or "+" after the whitespace is gone.
		if l != "" && strings.ContainsAny(l[:1], "+-") {
			c.add(l[1:])
		}
	}
	for _, e := range files {
		for _, r := range e.ranges {
			for _, l := range e.lines[r[0]-1 : r[1]] {
				c.add(l)
			}
		}
	}
	for _, pc := range related {
		for _, l := range pc.Lines {
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
	// Persona, if set, is the reviewer's prompt; Revise then rewrites the
	// intro and verdict in its voice. Leave it empty when the review's own
	// intro and verdict have already been shown (streamed output).
	Persona string
	// Prior is what earlier reviews of the same pull request raised (see
	// Input.Prior); nil outside a pull request.
	Prior []findings.Finding
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
	in := Input{
		Header:  r.ReviewDiff.ContextHeader,
		Diff:    r.Diff,
		Files:   files,
		Related: related(ctx, r.ReviewDiff, r.ReviewDiff.Diff, findingsIn(events), files),
		Prior:   r.Prior,
	}
	return Run(ctx, r.Client, in, events, r.startLog("check"))
}

// relatedLines caps the related code the check is shown. Aimed at the
// findings, it was 14-120 lines on Bark's own pull requests.
const relatedLines = 300

// related finds code in other files around the lines fs point to (see
// enclosing.Related). Files the check is already shown are left out.
func related(ctx context.Context, repo enclosing.Repo, diff string, fs []findings.Finding, files map[string]string) []enclosing.Piece {
	focus := map[string][]int{}
	for _, f := range fs {
		if f.File == "" || f.Line <= 0 {
			continue
		}
		for l := f.Line; l <= max(f.Line, f.EndLine); l++ {
			focus[f.File] = append(focus[f.File], l)
		}
	}
	if len(focus) == 0 {
		return nil
	}
	skip := make(map[string]bool, len(files))
	for p := range files {
		skip[p] = true
	}
	return enclosing.Related(ctx, diff, repo, enclosing.RelatedOptions{MaxLines: relatedLines, Focus: focus, Skip: skip})
}

// Revise rewrites the intro and verdict of a checked review in the
// persona's voice when the check doubted findings they may rest on (see the
// package function Revise). It reports whether it did; without a persona,
// or with nothing doubted, it makes no call.
func (r Review) Revise(ctx context.Context, events []findings.Event) ([]findings.Event, bool, error) {
	if r.Persona == "" || !NeedsRevision(events) {
		return events, false, nil
	}
	out, err := Revise(ctx, r.Client, r.Persona, events, r.startLog("verdict"))
	return out, err == nil, err
}

func (r Review) startLog(kind string) func(system, prompt string) *debuglog.Log {
	return func(system, prompt string) *debuglog.Log {
		log, err := debuglog.Start(r.LogDir, kind, debuglog.Meta{
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
	}
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
