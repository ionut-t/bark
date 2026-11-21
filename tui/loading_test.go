package tui

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The loading message changes on every rotation, and each round shows
// every message once.
func TestLoadingMessagePicker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		messages []string
	}{
		{"two", []string{"a", "b"}},
		{"three", []string{"one", "two", "three"}},
		{"many", []string{"a", "b", "c", "d", "e", "f", "g"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newLoadingMessagePicker(tt.messages)
			last := ""
			for round := range 2000 {
				var shown []string
				for range tt.messages {
					m := p.next()
					if m == last {
						t.Fatalf("round %d showed %q twice in a row", round, m)
					}
					shown, last = append(shown, m), m
				}
				slices.Sort(shown)
				want := slices.Sorted(slices.Values(tt.messages))
				if !assert.Equal(t, want, shown, "round %d", round) {
					return
				}
			}
		})
	}
}

func TestLoadingMessagePickerFew(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Loading...", newLoadingMessagePicker(nil).next())
	p := newLoadingMessagePicker([]string{"only"})
	assert.Equal(t, []string{"only", "only"}, []string{p.next(), p.next()})
}
