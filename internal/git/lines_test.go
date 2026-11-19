package git

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A git diff -U0 of a 12-line file: two lines inserted after line 1, line
// 6 changed, line 9 deleted, and a line inserted after the last one.
const zeroContextDiff = `diff --git a/f.go b/f.go
index 1111111..2222222 100644
--- a/f.go
+++ b/f.go
@@ -1,0 +2,2 @@ package f
+// one
+// two
@@ -6 +8 @@ func a() {
-	x := 1
+	x := 2
@@ -9 +10,0 @@ func b() {
-	gone()
@@ -12,0 +14 @@ func c() {
+// end
`

func TestLineMap(t *testing.T) {
	t.Parallel()

	m := parseLineMap(zeroContextDiff)
	tests := []struct {
		name       string
		start, end int
		want       [2]int
		ok         bool
	}{
		{"before every change", 1, 1, [2]int{1, 1}, true},
		{"shifted by the insertion", 2, 5, [2]int{4, 7}, true},
		{"a changed line", 6, 6, [2]int{}, false},
		{"a range around a changed line", 5, 7, [2]int{}, false},
		{"after a change of the same size", 7, 8, [2]int{9, 10}, true},
		{"a deleted line", 9, 9, [2]int{}, false},
		{"after the deletion", 10, 12, [2]int{11, 13}, true},
		{"an insertion inside the range", 12, 13, [2]int{}, false},
		{"an insertion right after the range doesn't touch it", 12, 12, [2]int{13, 13}, true},
		{"an insertion right before the range doesn't touch it", 2, 2, [2]int{4, 4}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			start, end, ok := m.Map(tt.start, tt.end)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.want, [2]int{start, end})
			}
		})
	}

	start, end, ok := parseLineMap("").Map(3, 4)
	assert.True(t, ok, "an unchanged file maps every line to itself")
	assert.Equal(t, [2]int{3, 4}, [2]int{start, end})
}

func TestGetLineMap(t *testing.T) {
	dir := newDivergedRepo(t)
	lines := func(n int, change map[int]string, insertTop ...string) string {
		var out []string
		out = append(out, insertTop...)
		for i := 1; i <= n; i++ {
			l := "line " + strings.Repeat("x", i)
			if c, ok := change[i]; ok {
				l = c
			}
			out = append(out, l)
		}
		return strings.Join(out, "\n") + "\n"
	}
	commitFile(t, dir, "f.txt", lines(10, nil))
	from := revParse(t, dir, "HEAD")
	commitFile(t, dir, "f.txt", lines(10, map[int]string{8: "changed"}, "new 1", "new 2"))
	to := revParse(t, dir, "HEAD")

	m, err := GetLineMap(t.Context(), from, to, "f.txt")
	require.NoError(t, err)
	start, end, ok := m.Map(4, 5)
	assert.True(t, ok)
	assert.Equal(t, [2]int{6, 7}, [2]int{start, end})
	_, _, ok = m.Map(7, 8)
	assert.False(t, ok, "line 8 changed")

	_, err = GetLineMap(t.Context(), strings.Repeat("0", 40), to, "f.txt")
	require.ErrorIs(t, err, ErrCommitMissing)
}
