package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/ionut-t/coffee/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An empty diff has nothing to review, so the TUI must say why instead of
// sending the model a prompt without a diff.
func TestReviewDiffLoadedEmptyDiff(t *testing.T) {
	ignored := []string{"CHANGELOG.md"}
	tests := []struct {
		name    string
		setup   func(m *Model)
		ignored []string
		want    string
	}{
		{name: "no uncommitted changes", want: "No uncommitted changes to review."},
		{
			name: "no staged changes", setup: func(m *Model) { m.stagedOnly = true },
			want: "No staged changes to review.",
		},
		{
			name: "branch without new commits", setup: func(m *Model) { m.branch = "main" },
			want: "No changes since main to review.",
		},
		{name: "empty commit", setup: func(m *Model) {
			m.selectCommit = true
			m.selectedCommit = &git.Commit{Hash: "abc1234def5678"}
		}, want: "Commit abc1234 has no changes to review."},
		{
			name: "empty pull request", setup: func(m *Model) { m.prNumber = "42" },
			want: "Pull request #42 has no changes to review.",
		},
		{
			name: "only ignored uncommitted changes", ignored: ignored,
			want: "The uncommitted changes touch only files Bark ignores (CHANGELOG.md), so there's nothing to review.",
		},
		{
			name: "only ignored staged changes", setup: func(m *Model) { m.stagedOnly = true }, ignored: ignored,
			want: "The staged changes touch only files Bark ignores (CHANGELOG.md), so there's nothing to review.",
		},
		{
			name: "only ignored branch changes", setup: func(m *Model) { m.branch = "main" }, ignored: ignored,
			want: "The commits since main change only files Bark ignores (CHANGELOG.md), so there's nothing to review.",
		},
		{
			name: "only ignored commit changes", setup: func(m *Model) {
				m.selectCommit = true
				m.selectedCommit = &git.Commit{Hash: "abc1234def5678"}
			}, ignored: ignored,
			want: "Commit abc1234 changes only files Bark ignores (CHANGELOG.md), so there's nothing to review.",
		},
		{
			name: "only ignored pull request changes", setup: func(m *Model) { m.prNumber = "42" },
			ignored: []string{"a.lock", "b.lock", "c.lock", "d.lock"},
			want:    "Pull request #42 changes only files Bark ignores (a.lock, b.lock, c.lock and 1 more), so there's nothing to review.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Model{
				config:           config.New("", config.Data{}),
				width:            200,
				height:           24,
				currentView:      viewInstructions,
				selectedReviewer: &reviewers.Reviewer{Name: "Spock"},
			}
			if tt.setup != nil {
				tt.setup(m)
			}
			_, _ = m.handleSelectedInstruction("")

			_, cmd := m.handleReviewDiffLoaded(reviewDiffLoadedMsg{reviewDiff: git.ReviewDiff{Ignored: tt.ignored}})

			assert.Nil(t, cmd, "nothing to review must not call the model")
			assert.Equal(t, viewInstructions, m.currentView)
			assert.False(t, m.preparing)
			view := m.createView()
			assert.Contains(t, view, tt.want)
			assert.Contains(t, view, "Press Esc to go back, or ctrl+c to exit.")
		})
	}
}

// Going back from the commit list and choosing another review option must
// not review commit "": git fails with "bad revision ”".
func TestReviewOptionAfterCancelledCommitSelection(t *testing.T) {
	var m tea.Model = Model{
		config:        config.New("", config.Data{}),
		width:         80,
		height:        24,
		currentView:   viewReviewOptions,
		reviewOptions: newReviewOptionsModel(styles.New(true), true),
	}
	m, _ = m.Update(reviewOptionSelectedMsg{option: ReviewOptionCommit})
	m, _ = m.Update(commitSelectedMsg{commit: git.Commit{Hash: "abc1234"}})
	require.True(t, asModel(m).selectCommit)

	m, _ = m.Update(cancelCommitSelectionMsg{})
	m, _ = m.Update(reviewOptionSelectedMsg{option: ReviewOptionCurrentChanges})

	assert.False(t, asModel(m).selectCommit, "the current changes, not a commit, are reviewed")
}

// The menu's review option decides whether only staged changes are reviewed,
// also after going back and picking another one.
func TestReviewOptionSetsStagedOnly(t *testing.T) {
	tests := []struct {
		name    string
		options []ReviewOption
		want    bool
	}{
		{name: "staged changes", options: []ReviewOption{ReviewOptionStagedChanges}, want: true},
		{name: "current changes", options: []ReviewOption{ReviewOptionCurrentChanges}, want: false},
		{name: "current changes after staged", options: []ReviewOption{ReviewOptionStagedChanges, ReviewOptionCurrentChanges}, want: false},
		{name: "staged after current changes", options: []ReviewOption{ReviewOptionCurrentChanges, ReviewOptionStagedChanges}, want: true},
		{name: "branch after staged", options: []ReviewOption{ReviewOptionStagedChanges, ReviewOptionBranch}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m tea.Model = Model{
				config:        config.New("", config.Data{}),
				width:         80,
				height:        24,
				currentView:   viewReviewOptions,
				reviewOptions: newReviewOptionsModel(styles.New(true), true),
			}
			for i, option := range tt.options {
				if i > 0 {
					m, _ = m.Update(cancelReviewerSelectionMsg{})
				}
				m, _ = m.Update(reviewOptionSelectedMsg{option: option})
			}

			assert.Equal(t, tt.want, asModel(m).stagedOnly)
		})
	}
}

// Going back from the review options to the task list must not leave a
// staged-only selection behind for the commit task, which then commits all
// changes again.
func TestCancelReviewOptionsResetsStagedOnly(t *testing.T) {
	var m tea.Model = Model{
		config:        config.New("", config.Data{}),
		width:         80,
		height:        24,
		currentView:   viewReviewOptions,
		reviewOptions: newReviewOptionsModel(styles.New(true), true),
		tasks:         newTasksModel(styles.New(true), true),
	}
	m, _ = m.Update(reviewOptionSelectedMsg{option: ReviewOptionStagedChanges})
	m, _ = m.Update(cancelReviewerSelectionMsg{})
	m, _ = m.Update(cancelReviewOptionSelectionMsg{})

	assert.Equal(t, viewTasks, asModel(m).currentView)
	assert.False(t, asModel(m).stagedOnly)
}

// asModel returns the Model that Update returned, by value or by pointer.
func asModel(m tea.Model) Model {
	if p, ok := m.(*Model); ok {
		return *p
	}
	return m.(Model)
}
