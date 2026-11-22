package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestItemFilterValue(t *testing.T) {
	assert.Equal(t, "Yoda", item{title: "Yoda", prompt: "p"}.FilterValue())
}
