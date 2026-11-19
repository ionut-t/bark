package prreview

import (
	"errors"
	"strings"

	"github.com/ionut-t/bark/v2/internal/findings"
)

// MapLines says where lines start..end of path at commit are now, and
// false if any of them changed since.
type MapLines func(commit, path string, start, end int) (int, int, bool, error)

// DismissedOnLines returns, by index in fs, the point dismissed on the
// pull request (in prior) whose lines each finding is on, when those lines
// haven't changed since the review that raised the point. It's a hint for
// Bark's check, which decides whether the finding makes the same point: the
// same problem is often raised again under another title, and a model
// doesn't always see it among many points.
//
// A point counts as dismissed if any copy of it (same file and title) was.
// Errors of lookups that failed are returned; those points are skipped.
func DismissedOnLines(fs []findings.Finding, prior []findings.Finding, mapLines MapLines) (map[int]findings.Finding, error) {
	key := func(p findings.Finding) string { return p.File + "\x00" + strings.ToLower(oneLine(p.Title)) }
	dismissedKeys := map[string]bool{}
	for _, p := range prior {
		if p.Dismissed != "" {
			dismissedKeys[key(p)] = true
		}
	}
	var points []findings.Finding
	for _, p := range prior {
		if dismissedKeys[key(p)] && p.Commit != "" && p.Line > 0 {
			points = append(points, p)
		}
	}
	if len(points) == 0 {
		return nil, nil
	}

	type span struct {
		start, end int
		ok         bool
	}
	mapped := make([]*span, len(points))
	var errs []error
	at := func(i int) span {
		if mapped[i] == nil {
			p := points[i]
			start, end, ok, err := mapLines(p.Commit, p.File, p.Line, max(p.EndLine, p.Line))
			if err != nil {
				errs = append(errs, err)
			}
			mapped[i] = &span{start, end, ok && err == nil}
		}
		return *mapped[i]
	}

	var out map[int]findings.Finding
	for j, f := range fs {
		if f.File == "" || f.Line <= 0 {
			continue
		}
		end := max(f.EndLine, f.Line)
		for i, p := range points {
			if p.File != f.File {
				continue
			}
			if s := at(i); s.ok && f.Line <= s.end && s.start <= end {
				if out == nil {
					out = map[int]findings.Finding{}
				}
				out[j] = p
				break
			}
		}
	}
	return out, errors.Join(errs...)
}
