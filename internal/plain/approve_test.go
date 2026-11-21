package plain

import (
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/prreview"
	"github.com/stretchr/testify/assert"
)

func TestApproval(t *testing.T) {
	t.Parallel()

	verdict := findings.Event{Verdict: "Ship it."}
	low := findings.Event{Finding: &findings.Finding{File: "a.go", Line: 1, Severity: findings.Low, Title: "Nit"}}
	high := findings.Event{Finding: &findings.Finding{File: "a.go", Line: 2, Severity: findings.High, Title: "Bug"}}
	doubted := findings.Event{Finding: &findings.Finding{File: "a.go", Line: 3, Severity: findings.Critical, Title: "No", Doubt: "Wrong."}}
	dismissed := findings.Event{Finding: &findings.Finding{File: "a.go", Line: 4, Severity: findings.Critical, Title: "Won't", Dismissed: "x"}}
	repeat := findings.Event{Finding: &findings.Finding{File: "a.go", Line: 5, Severity: findings.High, Title: "Again", RaisedBefore: "x"}}
	unchecked := findings.Event{Finding: &findings.Finding{File: "a.go", Line: 6, Severity: findings.High, Title: "?", Unchecked: "failed"}}
	whole := prreview.Scope{Head: "abc"}

	tests := []struct {
		name      string
		events    []findings.Event
		scope     prreview.Scope
		hist      prreview.History
		threshold findings.Severity
		want      bool
		why       string
	}{
		{name: "no findings", events: []findings.Event{verdict}, scope: whole, threshold: findings.Low, want: true},
		{name: "doubted and dismissed don't count", events: []findings.Event{doubted, dismissed, verdict}, scope: whole, threshold: findings.Low, want: true},
		{name: "below the threshold", events: []findings.Event{low, verdict}, scope: whole, threshold: findings.High, want: true},
		{name: "any finding blocks at low", events: []findings.Event{low, verdict}, scope: whole, threshold: findings.Low,
			why: "the review kept 1 finding(s) at or above low severity"},
		{name: "at the threshold", events: []findings.Event{high, verdict}, scope: whole, threshold: findings.High,
			why: "the review kept 1 finding(s) at or above high severity"},
		{name: "a repeat is still there", events: []findings.Event{repeat, verdict}, scope: whole, threshold: findings.High,
			why: "the review kept 1 finding(s) at or above high severity"},
		{name: "an unchecked finding counts", events: []findings.Event{unchecked, verdict}, scope: whole, threshold: findings.High,
			why: "the review kept 1 finding(s) at or above high severity"},
		{name: "partial", events: []findings.Event{verdict}, scope: prreview.Scope{Head: "abc", Partial: true}, threshold: findings.Low,
			why: "the review didn't see the whole change (the diff was cut to fit its limit, or the model's output ended early)"},
		{name: "no verdict", events: []findings.Event{{Intro: "Hi."}}, scope: whole, threshold: findings.Low,
			why: "the review has no verdict, so it may be incomplete"},
		{name: "nothing at all", scope: whole, threshold: findings.Low,
			why: "the review has no verdict, so it may be incomplete"},
		{name: "unreadable finding", events: []findings.Event{{Text: `{"file": "a.go", "title": "cut`}, verdict}, scope: whole, threshold: findings.Low,
			why: "Bark couldn't read one of the review's findings"},
		{name: "incremental with earlier open points", events: []findings.Event{verdict},
			scope:     prreview.Scope{Head: "abc", Since: "def"},
			hist:      prreview.History{Findings: []findings.Finding{{Title: "Old"}, {Title: "Gone", Dismissed: "👎"}}},
			threshold: findings.Low,
			why:       "it reviewed only the changes since the last review, and 1 point(s) raised earlier on the pull request may still be open (review the whole pull request to check them again, or dismiss them)"},
		{name: "incremental with earlier points all dismissed", events: []findings.Event{verdict},
			scope:     prreview.Scope{Head: "abc", Since: "def"},
			hist:      prreview.History{Findings: []findings.Finding{{Title: "Gone", Dismissed: "👎"}}},
			threshold: findings.Low, want: true},
		{name: "incremental with earlier points doubted", events: []findings.Event{verdict},
			scope:     prreview.Scope{Head: "abc", Since: "def"},
			hist:      prreview.History{Findings: []findings.Finding{{Title: "Nit", Doubt: "doubted"}}},
			threshold: findings.Low, want: true},
		{name: "whole review ignores earlier points", events: []findings.Event{verdict}, scope: whole,
			hist:      prreview.History{Findings: []findings.Finding{{Title: "Old"}}},
			threshold: findings.Low, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, why := approval(tt.events, tt.scope, tt.hist, tt.threshold)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.why, why)
		})
	}
}

func TestApprovalThreshold(t *testing.T) {
	t.Parallel()

	assert.Equal(t, findings.Low, approvalThreshold(""))
	assert.Equal(t, findings.High, approvalThreshold(findings.High))
}
