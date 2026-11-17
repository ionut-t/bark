package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ionut-t/bark/v2/internal/tokens"
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
			got, _ := truncateDiff(tt.diff, tt.limit)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTruncateDiff_KeepsUTF8Valid(t *testing.T) {
	t.Parallel()

	// "é" is 2 bytes; a 10-byte budget (4 tokens) falls inside the 6th one
	// only if the cut ignores rune boundaries.
	got, _ := truncateDiff(" "+strings.Repeat("é", 20)+"\n", DiffLimit{Tokens: 4})

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

// The cut says how many of the diff's files were kept, and whether the last
// one was kept whole.
func TestTruncateDiffReportsCut(t *testing.T) {
	t.Parallel()

	// Three files of three lines: a 13-byte header and two 3-byte lines,
	// 19 bytes a file.
	diff := "diff --git a\n+1\n+2\n" + "diff --git b\n+1\n+2\n" + "diff --git c\n+1\n+2\n"

	tests := []struct {
		name  string
		limit DiffLimit
		want  *DiffCut
	}{
		{name: "no limit", want: nil},
		{name: "whole diff fits", limit: DiffLimit{Tokens: 23}, want: nil}, // 57 bytes
		{name: "tokens: cut inside the second file", limit: DiffLimit{Tokens: 14}, want: &DiffCut{Files: 3, Kept: 2, Partial: true, Setting: "max_diff_tokens"}}, // 35 bytes
		{name: "tokens: cut at a file boundary", limit: DiffLimit{Tokens: 10}, want: &DiffCut{Files: 3, Kept: 1, Setting: "max_diff_tokens"}},                    // 25 bytes
		{name: "tokens: first line cut", limit: DiffLimit{Tokens: 2}, want: &DiffCut{Files: 3, Kept: 1, Partial: true, Setting: "max_diff_tokens"}},
		{name: "lines: cut inside the second file", limit: DiffLimit{Lines: 4}, want: &DiffCut{Files: 3, Kept: 2, Partial: true, Setting: "max_diff_lines"}},
		{name: "lines: cut at a file boundary", limit: DiffLimit{Lines: 6}, want: &DiffCut{Files: 3, Kept: 2, Setting: "max_diff_lines"}},
		{name: "lines: cut inside the first file", limit: DiffLimit{Lines: 1}, want: &DiffCut{Files: 3, Kept: 1, Partial: true, Setting: "max_diff_lines"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, cut := truncateDiff(diff, tt.limit)
			assert.Equal(t, tt.want, cut)
		})
	}
}

func TestDiffCutSummary(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Bark reviewed only the first 2 of 5 changed files, the last of them in part: the diff was cut to fit max_diff_tokens.",
		DiffCut{Files: 5, Kept: 2, Partial: true, Setting: "max_diff_tokens"}.Summary())
	assert.Equal(t, "Bark reviewed only the first 2 of 5 changed files: the diff was cut to fit max_diff_lines.",
		DiffCut{Files: 5, Kept: 2, Setting: "max_diff_lines"}.Summary())
	assert.Equal(t, "Bark reviewed only part of the 1 changed file: the diff was cut to fit max_diff_tokens.",
		DiffCut{Files: 1, Kept: 1, Partial: true, Setting: "max_diff_tokens"}.Summary())
	assert.Equal(t, "Bark reviewed only part of the diff: it was cut to fit max_diff_tokens.",
		DiffCut{Setting: "max_diff_tokens"}.Summary())
}

// GetReviewDiff reports a cut diff, whether it streams the diff (a branch)
// or cuts it whole (the working tree).
func TestGetReviewDiffReportsCut(t *testing.T) {
	dir := newDivergedRepo(t)
	small := "small\n"
	big := strings.Repeat("a line of the big file\n", 100)

	// The cut falls inside b.txt: a.txt and the start of b.txt fit.
	limitInB := func(full string) DiffLimit {
		at := strings.Index(full, "diff --git a/b.txt") + 200
		require.Less(t, at, strings.Index(full, "diff --git a/c.txt"))
		return DiffLimit{Tokens: uint32(float64(at) / tokens.CharsPerToken)}
	}
	want := &DiffCut{Files: 4, Kept: 2, Partial: true, Setting: "max_diff_tokens"}

	t.Run("branch", func(t *testing.T) {
		commitFile(t, dir, "a.txt", small)
		commitFile(t, dir, "b.txt", big)
		commitFile(t, dir, "c.txt", small)

		full, err := GetReviewDiff(t.Context(), BranchDiff("main"))
		require.NoError(t, err)
		assert.Nil(t, full.Cut, "an unlimited diff isn't cut")

		r, err := GetReviewDiff(t.Context(), BranchDiff("main").WithLimit(limitInB(full.Diff)))
		require.NoError(t, err)
		assert.Equal(t, want, r.Cut)
	})

	t.Run("working tree", func(t *testing.T) {
		for name, content := range map[string]string{"a.txt": "edit\n", "b.txt": strings.Repeat("a changed line of the big file\n", 100), "c.txt": "edit\n", "feature.txt": "edit\n"} {
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
		}

		full, err := GetReviewDiff(t.Context(), WorkingTreeDiff(false))
		require.NoError(t, err)
		assert.Nil(t, full.Cut)

		r, err := GetReviewDiff(t.Context(), WorkingTreeDiff(false).WithLimit(limitInB(full.Diff)))
		require.NoError(t, err)
		assert.Equal(t, want, r.Cut)
	})
}
