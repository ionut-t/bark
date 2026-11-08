package prompt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNumberDiffLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		diff string
		want string
	}{
		{
			name: "git diff",
			diff: "diff --git a/a.go b/a.go\n" +
				"--- a/a.go\n" +
				"+++ b/a.go\n" +
				"@@ -10,4 +10,5 @@ func f() {\n" +
				" \tx := 1\n" +
				"-\ty := 2\n" +
				"+\ty := 3\n" +
				"+\tz := 4\n" +
				"\n" +
				" }\n",
			want: "diff --git a/a.go b/a.go\n" +
				"--- a/a.go\n" +
				"+++ b/a.go\n" +
				"@@ -10,4 +10,5 @@ func f() {\n" +
				"    10  \tx := 1\n" +
				"       -\ty := 2\n" +
				"    11 +\ty := 3\n" +
				"    12 +\tz := 4\n" +
				"    13 \n" +
				"    14  }\n",
		},
		{
			// Without "diff --git" lines, the next file's "---" and "+++"
			// headers follow the hunk directly; the hunk counts tell them
			// apart from removed and added lines.
			name: "diff -u with two files",
			diff: "--- a/a.go\n" +
				"+++ b/a.go\n" +
				"@@ -1 +1 @@\n" +
				"-old\n" +
				"+new\n" +
				"--- a/b.go\n" +
				"+++ b/b.go\n" +
				"@@ -5,0 +6,2 @@\n" +
				"+one\n" +
				"+two\n" +
				"\\ No newline at end of file\n",
			want: "--- a/a.go\n" +
				"+++ b/a.go\n" +
				"@@ -1 +1 @@\n" +
				"       -old\n" +
				"     1 +new\n" +
				"--- a/b.go\n" +
				"+++ b/b.go\n" +
				"@@ -5,0 +6,2 @@\n" +
				"     6 +one\n" +
				"     7 +two\n" +
				"\\ No newline at end of file\n",
		},
		{
			name: "no hunks",
			diff: "Binary files a/x.png and b/x.png differ\n",
			want: "Binary files a/x.png and b/x.png differ\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, NumberDiffLines(tt.diff))
		})
	}
}
