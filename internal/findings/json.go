package findings

import (
	"strings"

	"github.com/ionut-t/bark/v2/internal/prices"
)

// ReportVersion is the version of Report's JSON shape. It goes up when a
// field is renamed or removed or changes meaning; adding a field doesn't
// change it.
const ReportVersion = 1

// Report is a structured review as JSON, what `bark review --format json`
// prints. Every finding is in it, the ones Bark's check set aside too,
// marked SetAside, so a reader can tell what the check kept.
type Report struct {
	Version int `json:"version"`
	// Intro and Verdict are the reviewer persona's words.
	Intro    string          `json:"intro,omitempty"`
	Findings []ReportFinding `json:"findings"`
	Verdict  string          `json:"verdict,omitempty"`
	// Text is model output that wasn't in the findings format.
	Text string `json:"text,omitempty"`
	// Checked is how many findings Bark's check gave a verdict on; 0 if it
	// didn't run or failed.
	Checked int `json:"checked"`
	// Truncated means the model reached its output limit, so the review
	// ends early and may be missing findings.
	Truncated bool `json:"truncated,omitempty"`
	// Partial says how much of a diff cut to fit its limit was reviewed;
	// empty if the whole diff was.
	Partial string `json:"partial,omitempty"`
	// Note says why there was nothing to review, when there wasn't.
	Note string `json:"note,omitempty"`
	// Cost is what the review's model calls cost; nil when there are no
	// prices for the model (no prices file, or Ollama) or no call was made.
	Cost *prices.Cost `json:"cost,omitempty"`
}

// ReportFinding is a finding with what Bark's check made of it.
type ReportFinding struct {
	File       string `json:"file"`
	Line       int    `json:"line,omitempty"`
	EndLine    int    `json:"endLine,omitempty"`
	Severity   string `json:"severity"`
	Category   string `json:"category,omitempty"`
	Title      string `json:"title"`
	Body       string `json:"body,omitempty"`
	Original   string `json:"original,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
	Remark     string `json:"remark,omitempty"`
	// SetAside is true when Bark's check set the finding aside: Doubt,
	// RaisedBefore or Dismissed says why. A set-aside finding isn't posted
	// and doesn't count for fail_on.
	SetAside bool `json:"setAside"`
	// Doubt is why Bark's check doubts the finding.
	Doubt string `json:"doubt,omitempty"`
	// RaisedBefore is the point an earlier review of the pull request
	// raised that this finding repeats.
	RaisedBefore string `json:"raisedBefore,omitempty"`
	// Dismissed is the point someone dismissed on the pull request that
	// this finding repeats.
	Dismissed string `json:"dismissed,omitempty"`
	// Unchecked is why Bark's check didn't check the finding. Unlike the
	// three above, it doesn't set the finding aside.
	Unchecked string `json:"unchecked,omitempty"`
	// Repaired means the model's line wasn't valid JSON and Bark fixed it,
	// so its fields may be cut short.
	Repaired bool `json:"repaired,omitempty"`
}

// NewReport returns the report of a structured review's events, checked
// being how many findings Bark's check gave a verdict on. A TruncatedNote
// event sets Truncated instead of adding to Text.
func NewReport(events []Event, checked int) Report {
	r := Report{Version: ReportVersion, Findings: []ReportFinding{}, Checked: checked}
	var text []string
	for _, e := range events {
		switch {
		case e.Intro != "":
			r.Intro = e.Intro
		case e.Verdict != "":
			r.Verdict = e.Verdict
		case e.Finding != nil:
			r.Findings = append(r.Findings, reportFinding(*e.Finding))
		case e.Text == TruncatedNote:
			r.Truncated = true
		case e.Text != "":
			text = append(text, e.Text)
		}
	}
	r.Text = strings.Join(text, "\n")
	return r
}

func reportFinding(f Finding) ReportFinding {
	return ReportFinding{
		File:         f.File,
		Line:         f.Line,
		EndLine:      f.EndLine,
		Severity:     string(f.Severity),
		Category:     f.Category,
		Title:        f.Title,
		Body:         f.Body,
		Original:     f.Original,
		Suggestion:   f.Suggestion,
		Remark:       f.Remark,
		SetAside:     f.SetAside(),
		Doubt:        f.Doubt,
		RaisedBefore: f.RaisedBefore,
		Dismissed:    f.Dismissed,
		Unchecked:    f.Unchecked,
		Repaired:     f.Repaired,
	}
}
