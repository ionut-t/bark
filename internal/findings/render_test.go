package findings

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPartialReviewNote(t *testing.T) {
	assert.Equal(t, "> **Partial review:** 3 of 5 files reviewed.\n", PartialReviewNote("3 of 5 files reviewed."))
}

func TestWarningNote(t *testing.T) {
	assert.Equal(t, "> **Warning:** could not read PR metadata\n", WarningNote("could not read PR metadata"))
}
