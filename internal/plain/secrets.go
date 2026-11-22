package plain

import (
	"fmt"
	"strings"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/redact"
)

// secretFindings returns Bark's own finding for each secret diff adds (see
// redact.Findings). prior are the points raised before on the pull
// request: a finding in the same file with the same title repeats one, and
// is marked raised before, or dismissed if that point was dismissed.
func secretFindings(diff string, prior []findings.Finding) []findings.Event {
	events := redact.Findings(diff)
	for _, e := range events {
		markRepeat(e.Finding, prior)
	}
	return events
}

// markRepeat marks f raised before, or dismissed, if it repeats a point in
// prior: the same file and title. A dismissal of any copy wins.
func markRepeat(f *findings.Finding, prior []findings.Finding) {
	for _, p := range prior {
		if p.File != f.File || !strings.EqualFold(strings.TrimSpace(p.Title), f.Title) {
			continue
		}
		point := fmt.Sprintf("%s (`%s`)", p.Title, p.File)
		if p.Line > 0 {
			point = fmt.Sprintf("%s (`%s:%d`)", p.Title, p.File, p.Line)
		}
		if p.Dismissed != "" {
			f.Dismissed, f.RaisedBefore = point, ""
			return
		}
		f.RaisedBefore = point
	}
}
