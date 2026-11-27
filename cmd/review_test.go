package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPickPreferredReviewer(t *testing.T) {
	t.Parallel()

	assert.Empty(t, pickPreferredReviewer(nil, func(int) int { panic("not called") }))

	names := []string{"Yoda", "Linus", "Bob"}
	for i, want := range names {
		got := pickPreferredReviewer(names, func(n int) int {
			assert.Equal(t, len(names), n, "every name can be picked")
			return i
		})
		assert.Equal(t, want, got)
	}
}

// The review action and Bark's own workflow read the picked reviewer from
// Bark's messages for their "Reviewed by" label, so they must match the line
// Bark prints.
func TestReviewActionReadsPickedReviewer(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"../.github/actions/bark-review/action.yaml",
		"../.github/workflows/bark.yaml",
	} {
		script, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(script), "'^"+pickedReviewerPrefix+"'", path)
		assert.Contains(t, string(script), "${PICKED#"+pickedReviewerPrefix+"}", path)
	}
}
