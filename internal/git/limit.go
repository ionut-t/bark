package git

import (
	"strings"

	"github.com/ionut-t/bark/v2/internal/tokens"
)

// truncatedMarker ends a diff that was cut to fit its limit.
const truncatedMarker = "... (truncated)\n"

// DiffLimit caps how much of a diff is sent to a model. A zero field sets
// no limit; with both set, the diff stops at whichever is reached first.
type DiffLimit struct {
	// Tokens is an estimate from the diff's size (see tokens.CharsPerToken).
	Tokens uint32
	// Lines is the line-count limit of the deprecated max_diff_lines
	// setting. A line can be any length, so a line count says little about
	// how much of the model's context a diff takes.
	Lines uint32
}

// LimitSettings are the settings a DiffLimit is read from; config.Config
// has them.
type LimitSettings interface {
	GetMaxDiffTokens() uint32
	GetMaxDiffLines() uint32
}

// LimitFrom returns the diff limit s sets.
func LimitFrom(s LimitSettings) DiffLimit {
	return DiffLimit{Tokens: s.GetMaxDiffTokens(), Lines: s.GetMaxDiffLines()}
}

// diffBudget tracks how much of a diff has been kept against its limit.
type diffBudget struct {
	limit DiffLimit
	lines uint32
	bytes int
}

// take returns the part of line (with its newline, if it has one) to keep,
// and whether the diff stops here. Whole lines are kept, except a first line
// longer than the whole token budget: that is cut, so the diff isn't empty.
func (b *diffBudget) take(line string) (string, bool) {
	if b.limit.Lines > 0 && b.lines >= b.limit.Lines {
		return "", true
	}

	if b.limit.Tokens > 0 {
		maxBytes := tokens.Bytes(b.limit.Tokens)
		if b.bytes+len(line) > maxBytes {
			if b.bytes > 0 {
				return "", true
			}
			// Dropping a rune cut in half keeps the prompt valid UTF-8.
			return strings.ToValidUTF8(line[:maxBytes], "") + "\n", true
		}
	}

	b.lines++
	b.bytes += len(line)
	return line, false
}

// truncateDiff cuts diff to limit, at a line end, and marks the cut.
func truncateDiff(diff string, limit DiffLimit) string {
	if limit == (DiffLimit{}) || diff == "" {
		return diff
	}

	budget := diffBudget{limit: limit}
	for start := 0; start < len(diff); {
		end := len(diff)
		if i := strings.IndexByte(diff[start:], '\n'); i >= 0 {
			end = start + i + 1
		}

		kept, stop := budget.take(diff[start:end])
		if stop {
			return diff[:start] + kept + truncatedMarker
		}
		start = end
	}

	return diff
}
