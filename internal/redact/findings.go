package redact

import (
	"fmt"

	"github.com/ionut-t/bark/v2/internal/findings"
)

// Findings returns a critical finding for each secret diff adds (see
// Added). Bark makes them itself, without the model, which only sees the
// secrets masked and is told not to report the placeholders.
func Findings(diff string) []findings.Event {
	var events []findings.Event
	for _, s := range Added(diff) {
		events = append(events, findings.Event{Finding: &findings.Finding{
			File:     s.File,
			Line:     s.Line,
			Severity: findings.Critical,
			Category: "security",
			Title:    fmt.Sprintf("Hard-coded secret (%s)", s.Kind),
			Body: fmt.Sprintf("This line adds what looks like a secret (%s). Bark masked it before sending the code to the model, "+
				"but anyone who can read this change can read it. If it has been pushed, treat it as leaked: revoke or rotate it, "+
				"then load it from the environment or a secret store instead of the code.", s.Kind),
		}})
	}
	return events
}
