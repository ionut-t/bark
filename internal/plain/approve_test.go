package plain

import (
	"errors"
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/houserules"
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
			why:       "it reviewed only the changes since the last review, and 1 point(s) raised earlier on the pull request may still be open (review the whole pull request to check them again, or dismiss or resolve them)"},
		{name: "incremental with earlier points all dismissed", events: []findings.Event{verdict},
			scope:     prreview.Scope{Head: "abc", Since: "def"},
			hist:      prreview.History{Findings: []findings.Finding{{Title: "Gone", Dismissed: "👎"}}},
			threshold: findings.Low, want: true},
		{name: "incremental with earlier points doubted", events: []findings.Event{verdict},
			scope:     prreview.Scope{Head: "abc", Since: "def"},
			hist:      prreview.History{Findings: []findings.Finding{{Title: "Nit", Doubt: "doubted"}}},
			threshold: findings.Low, want: true},
		{name: "incremental with earlier points resolved", events: []findings.Event{verdict},
			scope:     prreview.Scope{Head: "abc", Since: "def"},
			hist:      prreview.History{Findings: []findings.Finding{{Title: "Fixed", Resolved: "fixed in abc"}, {Title: "Gone", Dismissed: "👎"}}},
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

	tests := []struct {
		name         string
		approveBelow findings.Severity
		failOn       findings.Severity
		want         findings.Severity
		wantWarning  string
	}{
		{name: "default allows low nits", want: findings.Medium},
		{name: "set", approveBelow: findings.Low, want: findings.Low},
		{name: "fail_on above it changes nothing", approveBelow: findings.Medium, failOn: findings.High, want: findings.Medium},
		{name: "fail_on below the default wins quietly", failOn: findings.Low, want: findings.Low},
		{name: "fail_on below a set one wins with a warning", approveBelow: findings.Critical, failOn: findings.High, want: findings.High,
			wantWarning: "bark: warning: approve_below critical is above fail_on high; a review with a finding at high or above fails, so it doesn't approve either\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var w strings.Builder
			assert.Equal(t, tt.want, approvalThreshold(&w, tt.approveBelow, tt.failOn))
			assert.Equal(t, tt.wantWarning, w.String())
		})
	}
}

// A pull request that changes what Bark reads from its checkout to decide
// how to review it isn't approved: it could have set its own bar.
func TestSettingsApproval(t *testing.T) {
	t.Parallel()

	pointer := houserules.Rules{Targets: []string{"docs/rules.md"}}
	tests := []struct {
		name       string
		changed    []string
		listErr    error
		houseRules bool
		files      []string
		rules      houserules.Rules
		why        string
	}{
		{name: "code only", changed: []string{"main.go", "docs/bark.md", "pkg/.barkrc"}, houseRules: true},
		{name: "repo config", changed: []string{"main.go", ".bark/config.toml"},
			why: "the pull request changes .bark/config.toml, which Bark reads from the pull request's own checkout to decide how to review it; a person needs to approve it"},
		{name: "instructions, reviewer and ignore file", changed: []string{".bark/review.md", ".bark/reviewer.md", ".barkignore"},
			why: "the pull request changes .bark/review.md, .bark/reviewer.md and .barkignore, which Bark reads from the pull request's own checkout to decide how to review it; a person needs to approve it"},
		{name: "a .bark folder deeper down", changed: []string{"svc/.bark/review.md"},
			why: "the pull request changes svc/.bark/review.md, which Bark reads from the pull request's own checkout to decide how to review it; a person needs to approve it"},
		// A symlink in place of the folder: git lists the link, not
		// what's under it.
		{name: "a .bark symlink", changed: []string{".bark", "docs/x/review.md"},
			why: "the pull request changes .bark, which Bark reads from the pull request's own checkout to decide how to review it; a person needs to approve it"},
		{name: "a .bark symlink deeper down", changed: []string{"svc/.bark"},
			why: "the pull request changes svc/.bark, which Bark reads from the pull request's own checkout to decide how to review it; a person needs to approve it"},
		{name: "house rules", changed: []string{"pkg/AGENTS.md"}, houseRules: true,
			why: "the pull request changes pkg/AGENTS.md, which Bark reads from the pull request's own checkout to decide how to review it; a person needs to approve it"},
		{name: "house rules off", changed: []string{"AGENTS.md"}},
		{name: "a listed house-rules file", changed: []string{"docs/STYLE.md"}, houseRules: true, files: []string{"docs/STYLE.md"},
			why: "the pull request changes docs/STYLE.md, which Bark reads from the pull request's own checkout to decide how to review it; a person needs to approve it"},
		{name: "a file house rules point to", changed: []string{"docs/rules.md"}, houseRules: true, rules: pointer,
			why: "the pull request changes docs/rules.md, which Bark reads from the pull request's own checkout to decide how to review it; a person needs to approve it"},
		{name: "files unknown", listErr: errors.New("no commits"),
			why: "Bark couldn't list the files the pull request changes, to check it doesn't change Bark's own settings: no commits"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ok, why := settingsApproval(tt.changed, tt.listErr, tt.houseRules, tt.files, tt.rules)
			assert.Equal(t, tt.why == "", ok)
			assert.Equal(t, tt.why, why)
		})
	}
}
