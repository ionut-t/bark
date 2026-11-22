package prreview

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/github"
)

// client is what prreview asks of GitHub (github.Client). Tests replace
// it.
type client interface {
	PullRef(ctx context.Context, pr string) (github.PullRef, error)
	PostReview(ctx context.Context, pull github.Pull, review any) (string, error)
	DismissReview(ctx context.Context, pull github.Pull, id int64, message string) error
	Reviews(ctx context.Context, pull github.Pull) ([]github.Review, error)
	ReviewComments(ctx context.Context, pull github.Pull) ([]github.ReviewComment, error)
	Reply(ctx context.Context, pull github.Pull, id int64, body string) error
	ThumbsDown(ctx context.Context, repo github.Repo, id int64) ([]github.Reactor, error)
	CanWrite(ctx context.Context, repo github.Repo, login string) (bool, error)
	ReviewThreads(ctx context.Context, pull github.Pull) ([]github.ReviewThread, error)
	ResolveThread(ctx context.Context, id string) error
	UnresolveThread(ctx context.Context, id string) error
}

// Result says how a review was posted.
type Result struct {
	URL string
	// Inline is the number of findings posted as inline comments.
	Inline int
	// Fallback is true when GitHub rejected the inline comments and every
	// finding was posted in the review body instead.
	Fallback bool
	// Approved is true when the review was posted as an approval.
	Approved bool
	// ApprovalErr is why GitHub refused to approve the pull request, when
	// the review was to approve it but was posted as a comment instead.
	ApprovalErr error
}

// Post posts the events of a structured review of diff to pull request pr
// (a number, URL or branch, as gh accepts) as a review with inline
// comments. diff must be the diff the review was made from, so that only
// lines it shows get comments.
//
// scope says what the review covered. Its head, when set, is the commit
// the review is posted on and is recorded for the next review; otherwise
// the review goes on the pull request's current head and records none.
//
// With approve, the review approves the pull request; otherwise it's a
// comment.
//
// If GitHub rejects the review (for example because a line is no longer
// in the pull request), Post posts it again with every finding in the body.
// If GitHub refuses the approval (GitHub Actions isn't allowed to approve
// pull requests, or the pull request is the poster's own), the review is
// posted as a comment, and Result.ApprovalErr says why.
func Post(ctx context.Context, pr string, events []findings.Event, diff string, scope Scope, approve bool) (Result, error) {
	return post(ctx, github.New(), pr, events, diff, scope, approve)
}

func post(ctx context.Context, gh client, pr string, events []findings.Event, diff string, scope Scope, approve bool) (Result, error) {
	ref, err := lookUp(ctx, gh, pr)
	if err != nil {
		return Result{}, err
	}

	// The comments are on the lines of diff, which is of scope.Head: the
	// pull request may have moved on while the review ran.
	commit := scope.Head
	if commit == "" {
		commit = ref.HeadSHA
	}

	// Each attempt drops something GitHub may have rejected: first the
	// approval, then the inline comments, then both.
	kinds := []string{eventComment}
	if approve {
		kinds = []string{eventApprove, eventComment}
	}
	var errs []error
	var approvalErr error
	review := build(events, diff, scope)
	for _, withLines := range []bool{true, false} {
		if !withLines {
			if len(review.Comments) == 0 {
				break
			}
			review = build(events, "", scope)
		}
		review.CommitID = commit
		for _, event := range kinds {
			review.Event = event
			result, err := send(ctx, gh, ref.Pull, review)
			if err != nil {
				if event == eventApprove {
					// The last one: without inline comments, only the
					// approval can be what GitHub rejected.
					approvalErr = err
				}
				errs = append(errs, err)
				continue
			}
			result.Fallback = !withLines
			result.Approved = event == eventApprove
			if approve && !result.Approved {
				result.ApprovalErr = approvalErr
			}
			return result, nil
		}
	}
	if len(errs) == 1 {
		return Result{}, errs[0]
	}
	return Result{}, fmt.Errorf("failed to post review: %w", errors.Join(errs...))
}

// Earlier returns the findings that reviews Bark posted earlier on pull
// request pr (a number, URL or branch, as gh accepts) recorded, oldest
// first: the points already raised there. Reviews posted before Bark
// recorded findings add none.
func Earlier(ctx context.Context, pr string) ([]findings.Finding, error) {
	return earlier(ctx, github.New(), pr)
}

// trustedReviewer reports whether a review posted by a user of userType
// with association to the repository (GitHub's author_association) can be
// Bark's: one posted by a bot (GitHub Actions) or by someone with write
// access running Bark locally. Anyone can post a review whose body looks
// like Bark's, and its record decides which commits the next review skips
// and which findings it sets aside.
func trustedReviewer(userType, association string) bool {
	switch {
	case userType == "Bot":
		return true
	case association == "OWNER", association == "MEMBER", association == "COLLABORATOR":
		return true
	}
	return false
}

func earlier(ctx context.Context, gh client, pr string) ([]findings.Finding, error) {
	h, err := history(ctx, gh, pr)
	return h.Findings, err
}

// History is what the reviews Bark posted earlier on a pull request
// recorded.
type History struct {
	// Findings are the points they raised, oldest first.
	Findings []findings.Finding
	// Head is the commit the newest review that recorded one says the
	// next review can start from, or "" if none did.
	Head string
	// Base is the pull request's merge base that review recorded, if any.
	Base string
	// DismissalsErr is set when the inline comments, or who reacted to
	// them, couldn't all be read: some dismissed findings may lack
	// Dismissed. The rest of the history is still good.
	DismissalsErr error
	// Threads are the open threads of Bark's inline comments that nobody
	// answered or dismissed: the points Bark may resolve once its check
	// finds them fixed (see Resolve).
	Threads []Thread
	// ThreadsErr is set when the review threads couldn't be read: Threads
	// is empty, and no finding has Resolved.
	ThreadsErr error
	// Approvals are Bark's approving reviews still in force (not
	// dismissed), oldest first.
	Approvals []Approval
}

// Dismissed returns how many of h's findings were dismissed.
func (h History) Dismissed() int {
	n := 0
	for _, f := range h.Findings {
		if f.Dismissed != "" {
			n++
		}
	}
	return n
}

// ReadHistory returns what the reviews Bark posted earlier on pull request
// pr (a number, URL or branch, as gh accepts) recorded. Reviews posted by
// anyone but a bot or someone with write access are skipped.
func ReadHistory(ctx context.Context, pr string) (History, error) {
	return history(ctx, github.New(), pr)
}

func history(ctx context.Context, gh client, pr string) (History, error) {
	ref, err := lookUp(ctx, gh, pr)
	if err != nil {
		return History{}, err
	}
	pull := ref.Pull

	all, err := gh.Reviews(ctx, pull)
	if err != nil {
		return History{}, fmt.Errorf("failed to read earlier reviews: %w", err)
	}
	var h History
	var reviews []barkReview
	// Logins known to have write access, so their 👎 needs no lookup.
	trusted := map[string]bool{}
	for _, r := range all {
		// Who posted it decides whether it can be Bark's: see
		// trustedReviewer.
		if !strings.HasPrefix(r.Body, Marker) || !trustedReviewer(r.UserType, r.Association) {
			continue
		}
		// It's read back into git commands: take only a hash.
		if !commitHash.MatchString(r.CommitID) {
			r.CommitID = ""
		}
		fs := Recorded(r.Body)
		for i := range fs {
			fs[i].Commit = r.CommitID
		}
		reviews = append(reviews, barkReview{id: r.ID, commit: r.CommitID, findings: fs})
		if r.State == "APPROVED" {
			h.Approvals = append(h.Approvals, Approval{ID: r.ID, Commit: r.CommitID, pull: pull})
		}
		if s := RecordedScope(r.Body); s.Head != "" {
			h.Head, h.Base = s.Head, s.Base
		}
	}
	if len(reviews) == 0 {
		return h, nil
	}

	ids := map[int64]bool{}
	for _, r := range reviews {
		ids[r.id] = true
	}
	inline, err := readInline(ctx, gh, pull, ids, trusted)
	h.DismissalsErr = err
	for _, r := range reviews {
		h.Findings = append(h.Findings, markDismissed(r.findings, inline.dismissed[r.id], r.commit)...)
	}
	if len(inline.bark) > 0 {
		recorded := make(map[int64][]findings.Finding, len(reviews))
		for _, r := range reviews {
			recorded[r.id] = r.findings
		}
		h.ThreadsErr = h.readThreads(ctx, gh, pull, inline, recorded)
	}
	return h, nil
}

// lookUp finds pull request pr (a number, URL or branch, as gh accepts).
func lookUp(ctx context.Context, gh client, pr string) (github.PullRef, error) {
	ref, err := gh.PullRef(ctx, pr)
	if err != nil {
		return github.PullRef{}, fmt.Errorf("failed to look up pull request %s: %w", pr, err)
	}
	return ref, nil
}

func send(ctx context.Context, gh client, pull github.Pull, review Review) (Result, error) {
	url, err := gh.PostReview(ctx, pull, review)
	if err != nil {
		return Result{}, fmt.Errorf("failed to post review: %w", err)
	}
	return Result{URL: url, Inline: len(review.Comments)}, nil
}
