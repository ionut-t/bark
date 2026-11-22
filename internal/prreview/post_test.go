package prreview

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Post, Earlier and ReadHistory go through the gh on PATH.
func TestPostUsesGH(t *testing.T) {
	ctx := context.Background()

	t.Run("Post", func(t *testing.T) {
		log := failingGH(t)
		_, err := Post(ctx, "7", nil, "", Scope{}, false)
		assert.ErrorContains(t, err, "gh: boom")
		assert.Contains(t, calls(t, log), "pr view 7")
	})

	t.Run("Earlier", func(t *testing.T) {
		log := failingGH(t)
		_, err := Earlier(ctx, "7")
		assert.ErrorContains(t, err, "gh: boom")
		assert.Contains(t, calls(t, log), "pr view 7")
	})

	t.Run("ReadHistory", func(t *testing.T) {
		log := failingGH(t)
		_, err := ReadHistory(ctx, "7")
		assert.ErrorContains(t, err, "gh: boom")
		assert.Contains(t, calls(t, log), "pr view 7")
	})
}
