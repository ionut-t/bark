package prreview

import "github.com/ionut-t/bark/v2/internal/findings"

// NothingNew reports whether posting a review of scope with events would
// add nothing to what the newest review in last recorded: every finding is
// a repeat of a point raised before or dismissed (or there are none), and
// the review covers the same head and merge base that review recorded. A
// --full re-run on an unchanged pull request is the usual case. Such a
// review would only post another copy of the collapsed lists.
//
// A review whose head moved still adds something, even with only repeats:
// the head it records is where the next review starts. A partial review
// records where it started, so it doesn't count either.
func NothingNew(events []findings.Event, scope Scope, last History) bool {
	head := scope.recordedHead()
	if scope.Partial || head == "" || head != last.Head {
		return false
	}
	if scope.Base != "" && last.Base != "" && scope.Base != last.Base {
		return false
	}
	for _, e := range events {
		if f := e.Finding; f != nil && f.RaisedBefore == "" && f.Dismissed == "" {
			return false
		}
	}
	return true
}
