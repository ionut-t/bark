package prompt

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// NumberDiffLines prefixes every added and context line of a unified diff
// with its line number in the new file, so the model reads line numbers off
// the diff instead of counting from the hunk headers. Removed lines get a
// blank prefix of the same width; everything outside hunks is unchanged.
func NumberDiffLines(diff string) string {
	var b strings.Builder
	// newLine is the next new-side line number; oldLeft and newLeft are the
	// lines left in the current hunk on each side.
	var newLine, oldLeft, newLeft int

	lines := strings.SplitAfter(diff, "\n")
	for _, l := range lines {
		if m := hunkHeader.FindStringSubmatch(l); m != nil {
			oldLeft = count(m[1])
			newLine, _ = strconv.Atoi(m[2])
			newLeft = count(m[3])
			b.WriteString(l)
			continue
		}

		if oldLeft == 0 && newLeft == 0 {
			b.WriteString(l)
			continue
		}

		switch {
		case strings.HasPrefix(l, "-"):
			fmt.Fprintf(&b, "%6s %s", "", l)
			oldLeft--
		case strings.HasPrefix(l, "+"):
			fmt.Fprintf(&b, "%6d %s", newLine, l)
			newLine++
			newLeft--
		case strings.HasPrefix(l, " "), l == "\n":
			// A context line; some tools strip the space from blank ones.
			fmt.Fprintf(&b, "%6d %s", newLine, l)
			newLine++
			oldLeft--
			newLeft--
		default:
			// "\ No newline at end of file" and anything unexpected.
			b.WriteString(l)
		}
	}

	return b.String()
}

// count parses a hunk header's line count, which is 1 when omitted.
func count(s string) int {
	if s == "" {
		return 1
	}
	n, _ := strconv.Atoi(s)
	return n
}
