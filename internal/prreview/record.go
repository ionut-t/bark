package prreview

import (
	"encoding/json"
	"strings"

	"github.com/ionut-t/bark/v2/internal/findings"
)

// recordStart opens the hidden block at the end of a posted review that
// lists its findings, so a later review of the same pull request knows what
// was raised (see Recorded). The block is an HTML comment, which GitHub
// doesn't show.
const recordStart = "<!-- bark-findings v1"

// recorded is one finding as kept in a review's hidden block: enough to
// tell whether a later finding makes the same point.
type recorded struct {
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	EndLine int    `json:"end_line,omitempty"`
	Title   string `json:"title"`
	Body    string `json:"body,omitempty"`
}

// record renders the hidden block for every finding in events, posted or
// set aside, one JSON object per line. It's empty without findings.
func record(events []findings.Event) string {
	var b strings.Builder
	for _, e := range events {
		f := e.Finding
		if f == nil {
			continue
		}
		if b.Len() == 0 {
			b.WriteString(recordStart + "\n")
		}
		// json.Marshal escapes < and >, so no title or body can end the
		// comment early.
		line, err := json.Marshal(recorded{File: f.File, Line: f.Line, EndLine: f.EndLine, Title: f.Title, Body: findings.FirstSentence(f.Body)})
		if err != nil {
			continue
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if b.Len() > 0 {
		b.WriteString("-->\n")
	}
	return b.String()
}

// Recorded returns the findings listed in the hidden block of a review
// body Bark posted. Reviews posted before Bark recorded findings have none.
func Recorded(body string) []findings.Finding {
	// The block is the last thing in the body; a finding shown above it
	// could quote recordStart.
	i := strings.LastIndex(body, recordStart)
	if i < 0 {
		return nil
	}
	block, _, _ := strings.Cut(body[i+len(recordStart):], "-->")

	var out []findings.Finding
	for line := range strings.SplitSeq(block, "\n") {
		line = strings.TrimSpace(line)
		var r recorded
		if line == "" || json.Unmarshal([]byte(line), &r) != nil || r.File == "" || r.Title == "" {
			continue
		}
		out = append(out, findings.Finding{File: r.File, Line: r.Line, EndLine: r.EndLine, Title: r.Title, Body: r.Body})
	}
	return out
}
