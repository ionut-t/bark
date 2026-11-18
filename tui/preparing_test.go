package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cmdMsgs runs cmd and returns the messages it produces, flattening batches.
// Commands that take longer than a second, such as the 5s loading-message
// rotation, are left out so tests stay fast.
func cmdMsgs(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(time.Second):
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, cmdMsgs(t, c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func findMsg[T tea.Msg](msgs []tea.Msg) (T, bool) {
	for _, msg := range msgs {
		if v, ok := msg.(T); ok {
			return v, true
		}
	}
	var zero T
	return zero, false
}

// The spinner must show while git and the instructions load, not only once
// the LLM call starts; otherwise a slow diff leaves the previous screen up
// and the TUI looks frozen.
func TestSpinnerShowsWhileLoading(t *testing.T) {
	tests := []struct {
		name     string
		start    func(m *Model) tea.Cmd
		messages []string
		// done feeds the loaded data back and returns the next screen's
		// loader.
		done func(m *Model) loader
	}{
		{
			name: "review",
			start: func(m *Model) tea.Cmd {
				m.currentView = viewInstructions
				_, cmd := m.handleSelectedInstruction("")
				return cmd
			},
			messages: loadingMessages[:],
			done: func(m *Model) loader {
				m.selectedReviewer = &reviewers.Reviewer{Name: "Spock"}
				_, _ = m.handleReviewDiffLoaded(reviewDiffLoadedMsg{diff: "+a"})
				return m.review.loader
			},
		},
		{
			name: "commit",
			start: func(m *Model) tea.Cmd {
				m.currentView = viewTasks
				_, cmd := m.handleCommitMessage(false)
				return cmd
			},
			messages: generationPhaseLoadingMessages[:],
			done: func(m *Model) loader {
				_, _ = m.handleCommitDataLoaded(commitDataLoadedMsg{diff: "+a"})
				return m.commitChanges.loader
			},
		},
		{
			name: "pr",
			start: func(m *Model) tea.Cmd {
				m.currentView = viewPRDescriptionOptions
				_, cmd := m.handlePRDescription()
				return cmd
			},
			messages: prLoadingMessages[:],
			done: func(m *Model) loader {
				_, _ = m.handlePRDataLoaded(prDataLoadedMsg{content: "branch info"})
				assert.Equal(t, viewPRDescription, m.currentView)
				return m.pr.loader
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Model{config: config.New("", config.Data{}), width: 80, height: 24}

			cmd := tt.start(m)
			require.NotNil(t, cmd)
			first := m.loader.Message()
			assert.Contains(t, tt.messages, first, "loading should use the next screen's messages")
			assert.Contains(t, m.createView(), first, "spinner should show before the data loads")

			// The batch also holds the data loader, which would run git here.
			updated, next := m.Update(m.loader.spinner.Tick())
			require.NotNil(t, next, "the spinner should keep ticking while loading")
			*m = updated.(Model)

			updated, next = m.Update(rotation(m.loader))
			require.NotNil(t, next, "the message should keep rotating while loading")
			*m = updated.(Model)
			running := m.loader
			assert.Contains(t, tt.messages, running.Message())
			assert.NotEqual(t, first, running.Message(), "a rotation should show a different message")

			screen := tt.done(m)
			assert.False(t, m.preparing, "the loading screen should go once the data is in")
			assert.Equal(t, running.Message(), screen.Message(), "the next screen should keep the message on screen")
			assert.Equal(t, running.spinner.ID(), screen.spinner.ID(), "the next screen should keep the same spinner running")
			assert.True(t, screen.Owns(running.spinner.Tick()), "the loading screen's ticks should reach the next screen")
		})
	}
}

// Keys must not reach the screen under the spinner: pressing enter again on
// the instructions list would start a second load.
func TestPreparingIgnoresKeys(t *testing.T) {
	m := &Model{config: config.New("", config.Data{}), width: 80, height: 24, currentView: viewTasks}
	_, _ = m.handleCommitMessage(false)

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Nil(t, cmd)
}

// An empty commit diff while preparing from the review must leave the
// review on screen, so Esc goes back to it.
func TestPreparingCommitFromReviewKeepsReviewOnEmptyDiff(t *testing.T) {
	m := &Model{config: config.New("", config.Data{}), width: 80, height: 24, currentView: viewReview}
	_, _ = m.handleCommitMessage(false)
	_, _ = m.handleCommitDataLoaded(commitDataLoadedMsg{})

	assert.Equal(t, viewReview, m.currentView)
	assert.False(t, m.preparing)
}

// A rotation scheduled by an earlier load must not run alongside the new
// load's rotation, or the messages would change twice as often.
func TestPreparingDropsStaleRotation(t *testing.T) {
	m := &Model{config: config.New("", config.Data{}), width: 80, height: 24, currentView: viewReview}
	// The stale message passes on to the screen below, which ignores it too.
	m.review = newReviewModel(reviewers.Reviewer{Name: "Spock"}, "s", "p", 80, 24, nil)
	_, _ = m.handleCommitMessage(false)
	stale := rotation(m.loader)

	_, _ = m.handleCommitMessage(false)
	before := m.loader.Message()
	updated, cmd := m.Update(stale)
	assert.Nil(t, cmd, "the earlier load's rotation should end")
	assert.Equal(t, before, updated.(Model).loader.Message())
}
