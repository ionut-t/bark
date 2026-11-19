package git

import (
	"fmt"
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

// DiffCut says how much of a diff was kept when it was cut to fit its
// limit.
type DiffCut struct {
	// Files is the number of files the whole diff changes, and Kept the
	// number the cut diff shows; the last of them is cut in part if
	// Partial.
	Files, Kept int
	Partial     bool
	// PartialFile is the path of the file cut in part, if Partial and the
	// diff says.
	PartialFile string
	// Dropped are the paths of the files left out, in the order they
	// would have come next; "" for a file whose path the diff doesn't say.
	Dropped []string
	// Setting names the setting whose limit cut the diff.
	Setting string
}

// maxNamedFiles caps how many left-out files Summary names.
const maxNamedFiles = 10

// Summary tells the user how much of the change was reviewed, and which
// files were left out.
func (c DiffCut) Summary() string {
	switch c.Files {
	case 0:
		return "Bark reviewed only part of the diff: it was cut to fit " + c.Setting + "."
	case 1:
		return "Bark reviewed only part of the 1 changed file: the diff was cut to fit " + c.Setting + "."
	}
	s := fmt.Sprintf("Bark reviewed only %d of %d changed files", c.Kept, c.Files)
	if c.Partial {
		if c.PartialFile != "" {
			s += ", `" + c.PartialFile + "` in part"
		} else {
			s += ", one of them in part"
		}
	}
	s += ": the diff was cut to fit " + c.Setting + ", code first and tests, docs, fixtures and generated files last."
	if names := nameFiles(c.Dropped); names != "" {
		s += " Left out: " + names + "."
	}
	return s
}

// nameFiles lists up to maxNamedFiles of paths, and how many more there
// are. Paths that are "" are only counted.
func nameFiles(paths []string) string {
	var named []string
	for _, p := range paths {
		if p != "" && len(named) < maxNamedFiles {
			named = append(named, "`"+p+"`")
		}
	}
	more := len(paths) - len(named)
	switch {
	case len(named) == 0:
		return ""
	case more > 0:
		return strings.Join(named, ", ") + fmt.Sprintf(" and %d more", more)
	case len(named) == 1:
		return named[0]
	}
	return strings.Join(named[:len(named)-1], ", ") + " and " + named[len(named)-1]
}

// fileHeader starts each file's part of a diff.
const fileHeader = "diff --git "

// diffCutter cuts a diff fed to it line by line, and counts the files it
// keeps and the files in the whole diff.
type diffCutter struct {
	budget diffBudget
	cut    *DiffCut // nil until the diff is cut
	files  int
	kept   int
}

// line returns the part of line (with its newline, if it has one) to keep.
// After the cut it keeps nothing, but still counts the files.
func (c *diffCutter) line(line string) (kept string) {
	header := strings.HasPrefix(line, fileHeader)
	if header {
		c.files++
	}
	if c.cut != nil {
		return ""
	}
	kept, stop := c.budget.take(line)
	if header && kept != "" {
		c.kept++
	}
	if stop {
		// Cut right before a file's header, every file kept is whole.
		c.cut = &DiffCut{Partial: !header || kept != "", Setting: c.budget.hit}
	}
	return kept
}

// result returns the cut, or nil if the whole diff was kept. Call it once
// every line has been fed.
func (c *diffCutter) result() *DiffCut {
	if c.cut == nil {
		return nil
	}
	cut := *c.cut
	cut.Files, cut.Kept = c.files, c.kept
	return &cut
}

// diffBudget tracks how much of a diff has been kept against its limit.
type diffBudget struct {
	limit DiffLimit
	lines uint32
	bytes int
	// hit names the setting whose limit stopped the diff.
	hit string
}

// take returns the part of line (with its newline, if it has one) to keep,
// and whether the diff stops here. Whole lines are kept, except a first line
// longer than the whole token budget: that is cut, so the diff isn't empty.
func (b *diffBudget) take(line string) (string, bool) {
	if b.limit.Lines > 0 && b.lines >= b.limit.Lines {
		b.hit = "max_diff_lines"
		return "", true
	}

	if b.limit.Tokens > 0 {
		maxBytes := tokens.Bytes(b.limit.Tokens)
		if b.bytes+len(line) > maxBytes {
			b.hit = "max_diff_tokens"
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

// truncateDiff cuts diff to limit, at a line end, and marks the cut. The
// cut is nil if the whole diff fits, and the diff is then as it was. A diff
// that doesn't fit has its files ranked first (see pathTier), so code is
// kept before tests, docs, fixtures and generated files.
func truncateDiff(diff string, limit DiffLimit) (string, *DiffCut) {
	if limit == (DiffLimit{}) || diff == "" {
		return diff, nil
	}
	// Ranking doesn't change the diff's size, so a diff that fits in its
	// own order fits ranked.
	if _, cut := cutDiff(diff, limit); cut == nil {
		return diff, nil
	}

	ranked, files := rankFiles(diff)
	out, cut := cutDiff(ranked, limit)
	if cut.Partial && cut.Kept > 0 && cut.Kept <= len(files) {
		cut.PartialFile = files[cut.Kept-1].path
	}
	for _, f := range files[min(cut.Kept, len(files)):] {
		cut.Dropped = append(cut.Dropped, f.path)
	}
	return out, cut
}

// cutDiff is truncateDiff in the diff's own order, without naming files.
func cutDiff(diff string, limit DiffLimit) (string, *DiffCut) {
	c := diffCutter{budget: diffBudget{limit: limit}}
	var out string
	for start := 0; start < len(diff); {
		end := len(diff)
		if i := strings.IndexByte(diff[start:], '\n'); i >= 0 {
			end = start + i + 1
		}

		uncut := c.cut == nil
		kept := c.line(diff[start:end])
		if uncut && c.cut != nil {
			out = diff[:start] + kept + truncatedMarker
		}
		start = end
	}

	if cut := c.result(); cut != nil {
		return out, cut
	}
	return diff, nil
}
