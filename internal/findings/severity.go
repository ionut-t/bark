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

// Blocking returns the findings in events at or above threshold that Bark's
// check didn't doubt, and that don't repeat a point dismissed on the pull
// request ("won't fix"). A repeat of a point raised before on the pull request
// counts: it isn't posted again, but the model found it again in the code,
// so it's still there. An empty threshold blocks nothing.
func Blocking(events []Event, threshold Severity) []Finding {
	if threshold == "" {
		return nil
	}
	var out []Finding
	for _, e := range events {
		if e.Finding != nil && e.Finding.Doubt == "" && e.Finding.Dismissed == "" && e.Finding.Severity.AtLeast(threshold) {
			out = append(out, *e.Finding)
		}
	}
	return out
}
