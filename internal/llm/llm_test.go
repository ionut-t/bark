package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUsageThinkingTokens(t *testing.T) {
	tests := []struct {
		name  string
		usage Usage
		want  int64
	}{
		{name: "zero usage", usage: Usage{}, want: 0},
		{
			name:  "total equals input plus output",
			usage: Usage{InputTokens: 100, OutputTokens: 20, TotalTokens: 120},
			want:  0,
		},
		{
			name:  "gemini thinking left out of output",
			usage: Usage{InputTokens: 100, OutputTokens: 20, TotalTokens: 170},
			want:  50,
		},
		{
			name:  "cached tokens are part of input, not thinking",
			usage: Usage{InputTokens: 100, CachedInputTokens: 80, OutputTokens: 20, TotalTokens: 120},
			want:  0,
		},
		{
			name:  "total below input plus output never goes negative",
			usage: Usage{InputTokens: 100, OutputTokens: 20, TotalTokens: 50},
			want:  0,
		},
		{
			name:  "missing total never goes negative",
			usage: Usage{InputTokens: 100, OutputTokens: 20},
			want:  0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.usage.ThinkingTokens())
		})
	}
}
