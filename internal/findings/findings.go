// Package findings parses structured review output. The model streams JSON
// Lines: one object per finding, then a closing verdict. The parser turns
// each complete line into an Event as soon as it arrives, so a review can be
// rendered while it streams.
package findings

import (
	"encoding/json"
	"strings"
)

type Severity string

const (
	Critical Severity = "critical"
	High     Severity = "high"
	Medium   Severity = "medium"
	Low      Severity = "low"
)

// Finding is one issue in the reviewed diff. Every field except Remark is
// plain and neutral; Remark carries the reviewer persona's voice and is meant
// for people only.
type Finding struct {
	File string `json:"file"`
	// Line is the first line of the issue in the new version of File, or 0
	// if the issue isn't tied to a line.
	Line int `json:"line,omitempty"`
	// EndLine is the last line of the issue when it spans several lines.
	EndLine  int      `json:"end_line,omitempty"`
	Severity Severity `json:"severity"`
	Category string   `json:"category"`
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	// Suggestion is replacement code for Line..EndLine, exactly as it should
	// appear in the file.
	Suggestion string `json:"suggestion,omitempty"`
	Remark     string `json:"remark,omitempty"`
}

// Event is one parsed line of output. Exactly one field is set: the
// persona's Intro, a Finding, the persona's Verdict, or Text for output that
// isn't a valid object (shown as is, so a model that ignores the format still
// produces a readable review).
type Event struct {
	Intro   string
	Finding *Finding
	Verdict string
	Text    string
}

// Parser turns streamed chunks into Events. It isn't safe for concurrent use.
type Parser struct {
	partial string
}

// Feed adds a chunk of output and returns the Events for every line it
// completes. An unfinished line is kept until a later chunk or Flush.
func (p *Parser) Feed(chunk string) []Event {
	p.partial += chunk

	var events []Event
	for {
		line, ok := nextLine(&p.partial)
		if !ok {
			return events
		}
		if e, ok := parseLine(line); ok {
			events = append(events, e)
		}
	}
}

// Flush returns the Event for any unfinished last line. Call it once the
// output ends.
func (p *Parser) Flush() []Event {
	line := p.partial
	p.partial = ""
	if e, ok := parseLine(line); ok {
		return []Event{e}
	}
	return nil
}

// Parse parses a complete output.
func Parse(output string) []Event {
	var p Parser
	return append(p.Feed(output), p.Flush()...)
}

// line is the union of the intro, finding and verdict objects.
type line struct {
	Type string `json:"type"`
	Text string `json:"text"`
	Finding
}

// parseLine reports false for lines that carry nothing: blank lines, empty
// intros and verdicts, and the code fences models sometimes wrap JSON Lines
// in.
func parseLine(raw string) (Event, bool) {
	s := strings.TrimSpace(raw)
	if s == "" || strings.HasPrefix(s, "```") {
		return Event{}, false
	}

	var l line
	if !strings.HasPrefix(s, "{") || json.Unmarshal([]byte(s), &l) != nil {
		return Event{Text: strings.TrimRight(raw, "\r")}, true
	}

	switch l.Type {
	case "intro":
		intro := strings.TrimSpace(l.Text)
		return Event{Intro: intro}, intro != ""
	case "verdict":
		verdict := strings.TrimSpace(l.Text)
		return Event{Verdict: verdict}, verdict != ""
	}

	if l.Title == "" && l.Body == "" {
		return Event{Text: s}, true
	}
	f := normaliseFinding(l.Finding)
	return Event{Finding: &f}, true
}

func normaliseFinding(f Finding) Finding {
	f.Severity = normaliseSeverity(f.Severity)
	if f.EndLine <= f.Line {
		f.EndLine = 0
	}
	return f
}

// normaliseSeverity lowercases s and maps values outside the scale to
// Medium, so later filtering by severity has a known set to work with.
func normaliseSeverity(s Severity) Severity {
	switch v := Severity(strings.ToLower(strings.TrimSpace(string(s)))); v {
	case Critical, High, Medium, Low:
		return v
	default:
		return Medium
	}
}
