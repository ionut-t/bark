package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
