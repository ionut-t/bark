package tokens

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEstimate(t *testing.T) {
	tests := []struct {
		bytes int
		want  int
	}{
		{0, 0},
		{1, 0},
		{2, 0},
		{3, 1},
		{5, 2},
		{100, 40},
		{1001, 400},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, Estimate(tt.bytes), "Estimate(%d)", tt.bytes)
	}
}

func TestBytes(t *testing.T) {
	tests := []struct {
		tokens uint32
		want   int
	}{
		{0, 0},
		{1, 2},
		{2, 5},
		{40, 100},
		{1_000_000, 2_500_000},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, Bytes(tt.tokens), "Bytes(%d)", tt.tokens)
	}
}

func TestBytesAndEstimateRoundTrip(t *testing.T) {
	for _, n := range []uint32{0, 4, 40, 1000, 128_000} {
		assert.Equal(t, int(n), Estimate(Bytes(n)), "Estimate(Bytes(%d))", n)
	}
}
