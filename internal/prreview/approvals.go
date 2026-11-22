package prreview

import (
	"context"
	"errors"
	"fmt"

	"github.com/ionut-t/bark/v2/internal/github"
)

// Approval is one of Bark's approving reviews of a pull request that is
// still in force.
type Approval struct {
	ID int64
	// Commit is the commit it approved, if known.
	Commit string
	pull   github.Pull
}

// Withdraw dismisses Bark's earlier approvals once a review of commit head
// doesn't approve the pull request, saying why. On GitHub a later comment
// from the same reviewer doesn't replace an approval, so without this one
// would stand for code Bark no longer vouches for. It returns how many it
// dismissed; the errors for the rest are joined.
func Withdraw(ctx context.Context, approvals []Approval, head, why string) (int, error) {
	return withdraw(ctx, github.New(), approvals, head, why)
}

func withdraw(ctx context.Context, gh client, approvals []Approval, head, why string) (int, error) {
	message := fmt.Sprintf("Bark's review of %s doesn't approve this pull request: %s.", short(head), why)
	n := 0
	var errs []error
	for _, a := range approvals {
		if err := gh.DismissReview(ctx, a.pull, a.ID, message); err != nil {
			errs = append(errs, fmt.Errorf("failed to dismiss Bark's approval of %s: %w", short(a.Commit), err))
			continue
		}
		n++
	}
	return n, errors.Join(errs...)
}

// short is a commit's short hash, or "an unknown commit".
func short(commit string) string {
	if commit == "" {
		return "an unknown commit"
	}
	return commit[:min(shortHash, len(commit))]
}
