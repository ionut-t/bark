package plain

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/ionut-t/bark/v2/internal/check"
	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/ionut-t/bark/v2/internal/prreview"
)

// threadCode reads what an open thread's point was about and where that
// code is now.
type threadCode struct {
	// span says where lines start..end of path at commit, or the lines
	// that replaced them, are at the pull request's head, and whether any
	// of them changed.
	span func(commit, path string, start, end int) (int, int, bool, error)
	// file reads path at commit.
	file func(commit, path string) ([]byte, error)
}

// changedThreads returns the open threads whose code changed between the
// commit each was posted on and the pull request's head, and the same
// threads as Bark's check takes them. A point whose code didn't change
// can't have been fixed there, so it isn't checked. Threads that can't be
// compared are left open, and w says why.
func changedThreads(w io.Writer, threads []prreview.Thread, code threadCode) ([]prreview.Thread, []check.Thread) {
	var kept []prreview.Thread
	var toCheck []check.Thread
	failed := 0
	var errs []error
	fail := func(err error) {
		failed++
		// Threads on the same file and commit fail the same way.
		if !slices.ContainsFunc(errs, func(e error) bool { return e.Error() == err.Error() }) {
			errs = append(errs, err)
		}
	}
	for _, t := range threads {
		p := t.Point
		start, end, changed, err := code.span(p.Commit, p.File, p.Line, max(p.EndLine, p.Line))
		if err != nil {
			fail(err)
			continue
		}
		if !changed {
			continue
		}
		before, err := code.file(p.Commit, p.File)
		if err != nil {
			fail(err)
			continue
		}
		kept = append(kept, t)
		toCheck = append(toCheck, check.Thread{Point: p, Line: start, EndLine: end, Before: string(before)})
	}
	if failed > 0 {
		_, _ = fmt.Fprintf(w, "bark: warning: couldn't compare %d open thread(s) with the code now, so they're left open: %v\n",
			failed, errors.Join(errs...))
	}
	return kept, toCheck
}

// threadCodeAt reads the code of open threads with git, comparing it with
// head, which must be in the local repository like the commits the threads
// were posted on.
func threadCodeAt(maps *lineMaps) threadCode {
	return threadCode{
		span: func(commit, path string, start, end int) (int, int, bool, error) {
			m, err := maps.get(commit, path)
			if err != nil {
				return 0, 0, false, err
			}
			if s, e, ok := m.Map(start, end); ok {
				return s, e, false, nil
			}
			s, e := m.Span(start, end)
			return s, e, true, nil
		},
		file: func(commit, path string) ([]byte, error) {
			ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
			defer cancel()
			return git.GetFileContent(ctx, commit, path)
		},
	}
}

// fixedThreads returns the threads the check found fixed (by index, with
// its reason), in order.
func fixedThreads(threads []prreview.Thread, fixed map[int]string) []prreview.Fixed {
	var out []prreview.Fixed
	for i, t := range threads {
		if reason, ok := fixed[i]; ok {
			out = append(out, prreview.Fixed{Thread: t, Reason: reason})
		}
	}
	return out
}

// resolveFixed replies on and resolves the threads the check found fixed
// as of head, says on w how it went, and returns the ones it resolved.
func resolveFixed(w io.Writer, fixed []prreview.Fixed, head string) []prreview.Thread {
	if len(fixed) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	resolved, err := prreview.Resolve(ctx, fixed, head)
	if len(resolved) > 0 {
		titles := make([]string, len(resolved))
		for i, t := range resolved {
			titles[i] = fmt.Sprintf("%q", t.Point.Title)
		}
		_, _ = fmt.Fprintf(w, "bark: resolved %d thread(s) Bark's check found fixed: %s\n", len(resolved), strings.Join(titles, ", "))
	}
	if err != nil {
		_, _ = fmt.Fprintf(w, "bark: warning: couldn't resolve %d fixed thread(s): %v\n", len(fixed)-len(resolved), err)
	}
	return resolved
}

// threadsToCheck returns the open threads of hist whose code changed since
// they were posted, for Bark's check to say which are fixed, or none when
// it can't: the threads couldn't be read, the check is off, or the pull
// request's head is unknown. It says why on w when that matters.
func threadsToCheck(w io.Writer, hist prreview.History, checking bool, head string, maps *lineMaps) ([]prreview.Thread, []check.Thread) {
	if hist.ThreadsErr != nil {
		_, _ = fmt.Fprintf(w, "bark: warning: %v; fixed threads won't be resolved\n", hist.ThreadsErr)
		return nil, nil
	}
	if len(hist.Threads) == 0 {
		return nil, nil
	}
	if !checking {
		_, _ = fmt.Fprintln(w, "bark: fixed threads aren't resolved without Bark's check (--check)")
		return nil, nil
	}
	if head == "" {
		return nil, nil
	}
	return changedThreads(w, hist.Threads, threadCodeAt(maps))
}

// lineMaps maps lines from the commits of earlier reviews to head with
// git, reading each file's changes once.
type lineMaps struct {
	head    string
	lookups map[string]lineLookup
}

type lineLookup struct {
	m   git.LineMap
	err error
}

func newLineMaps(head string) *lineMaps {
	return &lineMaps{head: head, lookups: map[string]lineLookup{}}
}

// get returns the line map of path from commit to head. Failures are kept
// too, so a missing commit is looked up once.
func (l *lineMaps) get(commit, path string) (git.LineMap, error) {
	key := commit + "\x00" + path
	if lk, done := l.lookups[key]; done {
		return lk.m, lk.err
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	m, err := git.GetLineMap(ctx, commit, l.head, path)
	l.lookups[key] = lineLookup{m, err}
	return m, err
}

// maxOpenReason caps the reason printed for a thread left open.
const maxOpenReason = 300

// threadSummary says on w how many of threads res found fixed, and why
// each other one stays open, in order.
func threadSummary(w io.Writer, threads []check.Thread, res check.Result) {
	_, _ = fmt.Fprintf(w, "bark: the check found %d of %d open thread(s) fixed\n", len(res.Fixed), len(threads))
	for i, t := range threads {
		why, ok := res.Open[i]
		if !ok {
			continue
		}
		why = strings.Join(strings.Fields(why), " ")
		if len(why) > maxOpenReason {
			why = strings.ToValidUTF8(why[:maxOpenReason], "") + "…"
		}
		_, _ = fmt.Fprintf(w, "bark: left open: %q: %s\n", t.Point.Title, why)
	}
}
