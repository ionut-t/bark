package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/stretchr/testify/assert"
)

// Retrying within 5s of a failure used to leave the first attempt's
// message rotation running next to the retry's, so the message changed
// twice as often. The first attempt's ticks and messages must end.
func TestRetryEndsEarlierLoadingLoops(t *testing.T) {
	tests := []struct {
		name string
		// attempt fails the first attempt and starts the retry, returning
		// the loader of each.
		attempt func(t *testing.T) (first loader, retry func(tea.Msg) tea.Cmd)
	}{
		{
			name: "review",
			attempt: func(t *testing.T) (loader, func(tea.Msg) tea.Cmd) {
				m := newReviewModel(reviewers.Reviewer{Name: "Spock"}, "s", "p", 80, 24, nil)
				_ = m.startReview(t.Context())
				first := m.loader
				m, _ = m.Update(streamErrorMsg{error: errors.New("boom")})
				_ = m.startReview(t.Context())
				return first, func(msg tea.Msg) tea.Cmd { _, cmd := m.Update(msg); return cmd }
			},
		},
		{
			name: "pr",
			attempt: func(t *testing.T) (loader, func(tea.Msg) tea.Cmd) {
				m := newPRModel(nil, 80, 24)
				_ = m.startPRDescriptionGeneration(t.Context())
				first := m.loader
				m, _ = m.Update(prResponseMsg{error: errors.New("boom")})
				_ = m.startPRDescriptionGeneration(t.Context())
				return first, func(msg tea.Msg) tea.Cmd { _, cmd := m.Update(msg); return cmd }
			},
		},
		{
			name: "commit",
			attempt: func(t *testing.T) (loader, func(tea.Msg) tea.Cmd) {
				m := newCommitChangesModel(nil, "s", "p", false, 80, 24)
				_ = m.startCommitGeneration(t.Context())
				first := m.loader
				m, _ = m.Update(commitResponseMsg{error: errors.New("boom")})
				_ = m.startCommitGeneration(t.Context())
				return first, func(msg tea.Msg) tea.Cmd { _, cmd := m.Update(msg); return cmd }
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first, update := tt.attempt(t)
			assert.Nil(t, update(rotation(first)), "the first attempt's messages should end")
			assert.Nil(t, update(first.spinner.Tick()), "the first attempt's spinner loop should end")
		})
	}
}
