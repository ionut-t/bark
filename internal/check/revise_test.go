package check

import (
	"context"
	"errors"
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// checkedReview is a review after the check: one kept finding, one doubted.
func checkedReview() []findings.Event {
	return []findings.Event{
		{Intro: "A crash waiting to happen."},
		{Finding: &findings.Finding{File: "a.go", Line: 4, Severity: findings.High, Title: "Nil map", Body: "m is\nnil."}},
		{Finding: &findings.Finding{File: "b.go", Severity: findings.Low, Title: "Allocations", Body: "b", Doubt: "A preference."}},
		{Verdict: "Fix the nil map and the allocations."},
	}
}

func TestNeedsRevision(t *testing.T) {
	t.Parallel()

	kept := findings.Event{Finding: &findings.Finding{Title: "k"}}
	doubted := findings.Event{Finding: &findings.Finding{Title: "d", Doubt: "no"}}

	assert.True(t, NeedsRevision(checkedReview()))
	assert.False(t, NeedsRevision([]findings.Event{{Intro: "i"}, kept, {Verdict: "v"}}), "nothing doubted")
	assert.False(t, NeedsRevision([]findings.Event{doubted}), "no intro or verdict to rewrite")
	assert.True(t, NeedsRevision([]findings.Event{doubted, {Verdict: "v"}}))
}

func TestRevisePrompt(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "## Original intro\n\nA crash waiting to happen.\n\n"+
		"## Original verdict\n\nFix the nil map and the allocations.\n\n"+
		"## Findings that remain\n\n- [HIGH] Nil map (a.go): m is nil.\n"+
		"\n## Findings set aside\n\n- [LOW] Allocations (b.go)\n", RevisePrompt(checkedReview()))

	all := checkedReview()
	all[1].Finding.Doubt = "No."
	assert.Contains(t, RevisePrompt(all), "## Findings that remain\n\nNone.\n")
}

func TestRevise(t *testing.T) {
	t.Parallel()

	calls := 0
	client := fakeLLM{calls: &calls, output: `{"type":"intro","text":"Mostly fine."}` + "\n" +
		`{"type":"verdict","text":"Fix the nil map."}`}

	got, err := Revise(context.Background(), client, "You are Linus.", checkedReview(), nil)
	require.NoError(t, err)

	assert.Equal(t, 1, calls)
	want := checkedReview()
	want[0].Intro = "Mostly fine."
	want[3].Verdict = "Fix the nil map."
	assert.Equal(t, want, got)
}

func TestRevise_OnlyWhatTheModelWrote(t *testing.T) {
	t.Parallel()

	calls := 0
	client := fakeLLM{calls: &calls, output: "Sure!\n" + `{"type":"verdict","text":"Fix the nil map."}`}

	got, err := Revise(context.Background(), client, "You are Linus.", checkedReview(), nil)
	require.NoError(t, err)
	assert.Equal(t, "A crash waiting to happen.", got[0].Intro, "the intro is kept when there's no new one")
	assert.Equal(t, "Fix the nil map.", got[3].Verdict)
}

func TestRevise_NoCall(t *testing.T) {
	t.Parallel()

	nothingDoubted := checkedReview()
	nothingDoubted[2].Finding.Doubt = ""

	tests := []struct {
		name    string
		persona string
		events  []findings.Event
	}{
		{name: "nothing doubted", persona: "You are Linus.", events: nothingDoubted},
		{name: "no persona", persona: " ", events: checkedReview()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			calls := 0
			got, err := Revise(context.Background(), fakeLLM{calls: &calls}, tt.persona, tt.events, nil)
			require.NoError(t, err)
			assert.Equal(t, 0, calls)
			assert.Equal(t, tt.events, got)
		})
	}
}

// On a failed call or an answer without an intro or verdict, the review
// keeps its own.
func TestRevise_FailureKeepsOriginal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		client fakeLLM
	}{
		{name: "call fails", client: fakeLLM{err: errors.New("boom")}},
		{name: "no intro or verdict", client: fakeLLM{output: "I can't do that."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			calls := 0
			tt.client.calls = &calls
			got, err := Revise(context.Background(), tt.client, "You are Linus.", checkedReview(), nil)
			require.Error(t, err)
			assert.Equal(t, checkedReview(), got)
		})
	}
}

func TestReviseSystem(t *testing.T) {
	t.Parallel()

	got := ReviseSystem("You are Linus.")
	assert.Contains(t, got, `{"type":"verdict","text":"..."}`)
	assert.Contains(t, got, "# Persona\n\nYou are Linus.")
}
