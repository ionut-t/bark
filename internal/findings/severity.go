package findings

// rank orders severities; a value outside the scale counts as Medium.
func (s Severity) rank() int {
	switch normaliseSeverity(s) {
	case Critical:
		return 4
	case High:
		return 3
	case Low:
		return 1
	default:
		return 2
	}
}

// AtLeast reports whether s is as severe as min or more. An empty min
// is no threshold.
func (s Severity) AtLeast(min Severity) bool {
	return min == "" || s.rank() >= min.rank()
}

// DropBelow returns events without the findings less severe than min, and
// how many it left out. An empty min keeps every finding.
func DropBelow(events []Event, min Severity) ([]Event, int) {
	if min == "" {
		return events, 0
	}
	kept := make([]Event, 0, len(events))
	for _, e := range events {
		if e.Finding != nil && !e.Finding.Severity.AtLeast(min) {
			continue
		}
		kept = append(kept, e)
	}
	return kept, len(events) - len(kept)
}
