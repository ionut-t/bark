package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// ErrCommitMissing is returned when a commit isn't in the local repository
// (for example, a CI checkout without fetch-depth: 0).
var ErrCommitMissing = errors.New("commit isn't in the local repository")

// hunk is a hunk header of a diff without context: old lines
// oldStart..oldStart+oldCount-1 became newCount lines. With oldCount 0,
// the new lines were inserted after old line oldStart.
type hunk struct {
	oldStart, oldCount, newCount int
}

// LineMap says where the lines of a file at one commit are at a later one.
type LineMap struct {
	hunks []hunk
}

// GetLineMap returns where the lines of path at commit from are at commit
// to. Both must be in the local repository.
func GetLineMap(ctx context.Context, from, to, path string) (LineMap, error) {
	for _, c := range []string{from, to} {
		if !hasCommit(ctx, c) {
			return LineMap{}, fmt.Errorf("%w: %s", ErrCommitMissing, c[:min(7, len(c))])
		}
	}
	out, err := exec.CommandContext(ctx, "git", "diff", "-U0", "--no-color", "--no-ext-diff", "--no-textconv", "--no-renames",
		from, to, "--", path).Output()
	if err != nil {
		return LineMap{}, fmt.Errorf("failed to diff %s: %w", path, err)
	}
	return parseLineMap(string(out)), nil
}

// hunkHeader matches a hunk header: old start and count, new start and
// count (a missing count is 1).
var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

func count(s string) int {
	if s == "" {
		return 1
	}
	n, _ := strconv.Atoi(s)
	return n
}

func parseLineMap(diff string) LineMap {
	var m LineMap
	for l := range strings.Lines(diff) {
		h := hunkHeader.FindStringSubmatch(l)
		if h == nil {
			continue
		}
		start, _ := strconv.Atoi(h[1])
		m.hunks = append(m.hunks, hunk{oldStart: start, oldCount: count(h[2]), newCount: count(h[4])})
	}
	return m
}

// Map returns where lines start..end are at the later commit, and false
// if any of them changed (or new lines were inserted between them).
func (m LineMap) Map(start, end int) (int, int, bool) {
	offset := 0
	for _, h := range m.hunks {
		if h.oldCount == 0 {
			if start <= h.oldStart && h.oldStart < end {
				return 0, 0, false
			}
			if h.oldStart < start {
				offset += h.newCount
			}
			continue
		}
		last := h.oldStart + h.oldCount - 1
		if h.oldStart <= end && start <= last {
			return 0, 0, false
		}
		if last < start {
			offset += h.newCount - h.oldCount
		}
	}
	return start + offset, end + offset, true
}
