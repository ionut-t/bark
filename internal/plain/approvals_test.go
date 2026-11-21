package plain

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/ionut-t/bark/v2/internal/prreview"
	"github.com/stretchr/testify/assert"
)

// A review that doesn't approve withdraws Bark's earlier approvals and
// says so; with none, nothing is called.
func TestWithdrawApprovals(t *testing.T) {
	t.Parallel()

	approvals := []prreview.Approval{{ID: 1, Commit: "aaaaaaa1"}, {ID: 2, Commit: "bbbbbbb2"}}
	tests := []struct {
		name      string
		approvals []prreview.Approval
		n         int
		err       error
		want      string
		called    bool
	}{
		{name: "none", want: ""},
		{name: "withdrawn", approvals: approvals, n: 2, called: true,
			want: "bark: withdrew Bark's 2 earlier approval(s): it found blocking findings\n"},
		{name: "partly", approvals: approvals, n: 1, err: errors.New("failed to dismiss Bark's approval of bbbbbbb: HTTP 403"), called: true,
			want: "bark: withdrew Bark's 1 earlier approval(s): it found blocking findings\n" +
				"bark: warning: couldn't withdraw 1 earlier approval(s), so they still approve the pull request: failed to dismiss Bark's approval of bbbbbbb: HTTP 403\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			called := false
			withdrawApprovals(&out, tt.approvals, "ccccccc3", "it found blocking findings",
				func(_ context.Context, got []prreview.Approval, head, why string) (int, error) {
					called = true
					assert.Equal(t, tt.approvals, got)
					assert.Equal(t, "ccccccc3", head)
					assert.Equal(t, "it found blocking findings", why)
					return tt.n, tt.err
				})
			assert.Equal(t, tt.called, called)
			assert.Equal(t, tt.want, out.String())
		})
	}
}
