package plain

import (
	"context"
	"fmt"
	"io"

	"github.com/ionut-t/bark/v2/internal/prreview"
)

// withdrawFunc dismisses Bark's earlier approvals (prreview.Withdraw).
type withdrawFunc func(ctx context.Context, approvals []prreview.Approval, head, why string) (int, error)

// withdrawApprovals dismisses Bark's earlier approvals of a pull request
// once a review of head doesn't approve it, for why, and says so on w.
func withdrawApprovals(w io.Writer, approvals []prreview.Approval, head, why string, withdraw withdrawFunc) {
	if len(approvals) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	n, err := withdraw(ctx, approvals, head, why)
	if n > 0 {
		_, _ = fmt.Fprintf(w, "bark: withdrew Bark's %d earlier approval(s): %s\n", n, why)
	}
	if err != nil {
		_, _ = fmt.Fprintf(w, "bark: warning: couldn't withdraw %d earlier approval(s), so they still approve the pull request: %v\n",
			len(approvals)-n, err)
	}
}
