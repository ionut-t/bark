package tui

import (
	"fmt"
	"os"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/ionut-t/bark/v2/internal/instructions"
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

// The window size arrives once at startup, before the reviewers and
// instructions load, so their lists must take the known size when built
// instead of staying at the default 25 lines.
func TestListsFollowWindowSize(t *testing.T) {
	const count = 30

	t.Run("reviewers", func(t *testing.T) {
		rs := make([]reviewers.Reviewer, count)
		for i := range rs {
			rs[i] = reviewers.Reviewer{Name: fmt.Sprintf("Reviewer %02d", i)}
		}
		var m tea.Model = Model{config: config.New("", config.Data{}), styles: styles.New(true)}
		m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 60})
		m, _ = m.Update(reviewersLoadedMsg{reviewers: rs, fileErr: os.ErrNotExist})

		got := asModel(m)
		require.Equal(t, viewReviewers, got.currentView)
		assert.Equal(t, 56, got.reviewers.list.Height())
		assert.Equal(t, 120, got.reviewers.list.Width())
		assert.Equal(t, 1, got.reviewers.list.Paginator.TotalPages, "every reviewer fits on one page")
	})

	t.Run("instructions", func(t *testing.T) {
		is := make([]instructions.Instruction, count)
		for i := range is {
			is[i] = instructions.Instruction{Name: fmt.Sprintf("Instruction %02d", i)}
		}
		var m tea.Model = Model{config: config.New("", config.Data{}), styles: styles.New(true)}
		m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 60})
		m, _ = m.Update(reviewInstructionsLoadedMsg{instructions: is})

		got := asModel(m)
		require.Equal(t, viewInstructions, got.currentView)
		assert.Equal(t, 56, got.instructions.list.Height())
		assert.Equal(t, 120, got.instructions.list.Width())
		assert.Equal(t, 1, got.instructions.list.Paginator.TotalPages, "every instruction fits on one page")
	})
}

// A resize must reach every list that has been built, not only the one on
// screen: the fixed lists built at startup and the lists you go back to would
// otherwise keep their old size and, in a short terminal, lose their title.
func TestResizeSizesEveryBuiltList(t *testing.T) {
	s := styles.New(true)
	var m tea.Model = Model{
		config:               config.New("", config.Data{}),
		styles:               s,
		width:                80,
		height:               24,
		currentView:          viewReviewers,
		tasks:                newTasksModel(s, true),
		reviewOptions:        newReviewOptionsModel(s, true),
		prDescriptionOptions: newPRDescriptionOptionsModel(s, true),
		commits:              newCommitsModel([]git.Commit{{Hash: "abc1234"}}),
		reviewers:            newReviewersModel([]reviewers.Reviewer{{Name: "Spock"}}, s, true),
		instructions:         newInstructionsModel([]instructions.Instruction{{Name: "Go"}}, s, true),
	}
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})

	got := asModel(m)
	for name, l := range map[string]list.Model{
		"tasks":                  got.tasks.list,
		"review options":         got.reviewOptions.list,
		"PR description options": got.prDescriptionOptions.list,
		"commits":                got.commits.list,
		"reviewers":              got.reviewers.list,
		"instructions":           got.instructions.list,
	} {
		assert.Equal(t, 100, l.Width(), name)
		assert.Equal(t, 16, l.Height(), name)
	}
}

// Lists not built yet are zero values; a resize must not touch them.
func TestResizeBeforeListsLoad(t *testing.T) {
	var m tea.Model = Model{config: config.New("", config.Data{}), currentView: viewInit}

	assert.NotPanics(t, func() { m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 20}) })
	assert.Equal(t, 100, asModel(m).width)
}

// The resize message is also passed on to the view on screen; that view must
// not undo the room the app leaves for the header (height - 4).
func TestResizeOnScreenListKeepsHeaderRoom(t *testing.T) {
	s := styles.New(true)
	tests := []struct {
		name   string
		view   view
		height func(Model) int
	}{
		{"commits", viewCommits, func(m Model) int { return m.commits.list.Height() }},
		{"instructions", viewInstructions, func(m Model) int { return m.instructions.list.Height() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m tea.Model = Model{
				config:       config.New("", config.Data{}),
				styles:       s,
				currentView:  tt.view,
				commits:      newCommitsModel([]git.Commit{{Hash: "abc1234"}}),
				instructions: newInstructionsModel([]instructions.Instruction{{Name: "Go"}}, s, true),
			}
			m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

			assert.Equal(t, 26, tt.height(asModel(m)))
		})
	}
}

// asModel returns the Model that Update returned, by value or by pointer.
func asModel(m tea.Model) Model {
	if p, ok := m.(*Model); ok {
		return *p
	}
	return m.(Model)
}
