package git

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTruncateDiff(t *testing.T) {
	t.Parallel()

	// Each line is 10 bytes with its newline, so 4 tokens (10 bytes at 2.5
	// bytes a token) hold exactly one line.
	diff := "line-0001\nline-0002\nline-0003\n"

	tests := []struct {
		name  string
		diff  string
		limit DiffLimit
		want  string
	}{
		{name: "no limit", diff: diff, want: diff},
		{name: "empty diff", diff: "", limit: DiffLimit{Tokens: 1}, want: ""},
		{name: "tokens: whole diff fits", diff: diff, limit: DiffLimit{Tokens: 12}, want: diff},
		{name: "tokens: cut at a line end", diff: diff, limit: DiffLimit{Tokens: 9}, want: "line-0001\nline-0002\n" + truncatedMarker},
		{name: "tokens: exactly one line", diff: diff, limit: DiffLimit{Tokens: 4}, want: "line-0001\n" + truncatedMarker},
		{
			name:  "tokens: a first line longer than the budget is cut, not dropped",
			diff:  strings.Repeat("x", 100) + "\nnext\n",
			limit: DiffLimit{Tokens: 4},
			want:  strings.Repeat("x", 10) + "\n" + truncatedMarker,
		},
		{name: "tokens: no trailing newline", diff: "line-0001\nlast", limit: DiffLimit{Tokens: 100}, want: "line-0001\nlast"},
		{name: "lines: cut", diff: diff, limit: DiffLimit{Lines: 2}, want: "line-0001\nline-0002\n" + truncatedMarker},
		{name: "lines: exactly the limit is not truncated", diff: diff, limit: DiffLimit{Lines: 3}, want: diff},
		{name: "both: tokens cut first", diff: diff, limit: DiffLimit{Lines: 3, Tokens: 4}, want: "line-0001\n" + truncatedMarker},
		{name: "both: lines cut first", diff: diff, limit: DiffLimit{Lines: 1, Tokens: 100}, want: "line-0001\n" + truncatedMarker},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, truncateDiff(tt.diff, tt.limit))
		})
	}
}

func TestTruncateDiff_KeepsUTF8Valid(t *testing.T) {
	t.Parallel()

	// "é" is 2 bytes; a 10-byte budget (4 tokens) falls inside the 6th one
	// only if the cut ignores rune boundaries.
	got := truncateDiff(" "+strings.Repeat("é", 20)+"\n", DiffLimit{Tokens: 4})

	assert.True(t, utf8.ValidString(got), "cut mid-rune: %q", got)
	assert.Equal(t, " "+strings.Repeat("é", 4)+"\n"+truncatedMarker, got)
}

func TestGetBranchDiff_TokenLimit(t *testing.T) {
	dir := newDivergedRepo(t)
	commitFile(t, dir, "big.txt", strings.Repeat("a line of the big file\n", 1000))

	full, err := GetBranchDiff(t.Context(), "main", DiffLimit{})
	require.NoError(t, err)

	const limit = 200
	diff, err := GetBranchDiff(t.Context(), "main", DiffLimit{Tokens: limit})
	require.NoError(t, err)

	assert.True(t, strings.HasSuffix(diff, truncatedMarker), "diff should say it was cut")
	body := strings.TrimSuffix(diff, truncatedMarker)
	assert.LessOrEqual(t, len(body), int(limit*2.5), "diff should fit the token budget")
	assert.True(t, strings.HasPrefix(full, body), "the cut diff should be a prefix of the full one")
	assert.True(t, strings.HasSuffix(body, "\n"), "cut at a line end")
}
