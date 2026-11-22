package plain

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/check"
	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/prreview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const postedOn = "c0ffee1234567890c0ffee1234567890c0ffee12"

func openThread(id, file string, line int) prreview.Thread {
	return prreview.Thread{ID: id, Point: findings.Finding{File: file, Line: line, EndLine: line + 1, Title: id, Commit: postedOn}}
}

// Only threads whose code changed since they were posted are checked: an
// untouched point can't have been fixed there.
func TestChangedThreads(t *testing.T) {
	t.Parallel()

	threads := []prreview.Thread{
		openThread("changed", "a.go", 10),
		openThread("untouched", "a.go", 20),
		openThread("missing commit", "b.go", 5),
		openThread("unreadable", "c.go", 5),
	}
	code := threadCode{
		span: func(commit, path string, start, end int) (int, int, bool, error) {
			assert.Equal(t, postedOn, commit)
			switch {
			case path == "b.go":
				return 0, 0, false, errors.New("commit isn't in the local repository: c0ffee1")
			case start == 20:
				return 22, 23, false, nil
			}
			return start + 2, end + 5, true, nil
		},
		file: func(commit, path string) ([]byte, error) {
			if path == "c.go" {
				return nil, errors.New("gone")
			}
			return []byte("old " + path), nil
		},
	}
	var w strings.Builder
	kept, toCheck := changedThreads(&w, threads, code)

	assert.Equal(t, []prreview.Thread{threads[0]}, kept)
	assert.Equal(t, []check.Thread{{Point: threads[0].Point, Line: 12, EndLine: 16, Before: "old a.go"}}, toCheck)
	assert.Equal(t, "bark: warning: couldn't compare 2 open thread(s) with the code now, so they're left open: "+
		"commit isn't in the local repository: c0ffee1\ngone\n", w.String())
}

func TestFixedThreads(t *testing.T) {
	t.Parallel()

	threads := []prreview.Thread{openThread("a", "a.go", 1), openThread("b", "a.go", 5), openThread("c", "a.go", 9)}
	got := fixedThreads(threads, map[int]string{2: "Third.", 0: "First."})
	assert.Equal(t, []prreview.Fixed{{Thread: threads[0], Reason: "First."}, {Thread: threads[2], Reason: "Third."}}, got)
}

// Threads on the same file and commit fail the same way; the warning
// counts them all but says why once.
func TestChangedThreadsSameErrorOnce(t *testing.T) {
	t.Parallel()

	code := threadCode{span: func(string, string, int, int) (int, int, bool, error) {
		return 0, 0, false, errors.New("commit isn't in the local repository: c0ffee1")
	}}
	var w strings.Builder
	kept, _ := changedThreads(&w, []prreview.Thread{openThread("a", "a.go", 1), openThread("b", "a.go", 9)}, code)
	assert.Empty(t, kept)
	assert.Equal(t, "bark: warning: couldn't compare 2 open thread(s) with the code now, so they're left open: "+
		"commit isn't in the local repository: c0ffee1\n", w.String())
}

// After the check, the run says how many threads it found fixed and why
// each other one stays open, one line each, in order.
func TestThreadSummary(t *testing.T) {
	t.Parallel()

	threads := []check.Thread{
		{Point: findings.Finding{Title: "Nil map"}},
		{Point: findings.Finding{Title: "Non-atomic"}},
		{Point: findings.Finding{Title: "Ellipsis"}},
	}
	var out bytes.Buffer
	threadSummary(&out, threads, check.Result{
		Fixed: map[int]string{0: "Checked now."},
		Open: map[int]string{
			2: "the check gave no verdict",
			1: "Resolving and replying\n  are still two calls. " + strings.Repeat("x", 400),
		},
	})
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, "bark: the check found 1 of 3 open thread(s) fixed", lines[0])
	assert.True(t, strings.HasPrefix(lines[1], `bark: left open: "Non-atomic": Resolving and replying are still two calls. xxx`), lines[1])
	assert.True(t, strings.HasSuffix(lines[1], "x…"), "a long reason is cut")
	assert.Less(t, len(lines[1]), 360)
	assert.Equal(t, `bark: left open: "Ellipsis": the check gave no verdict`, lines[2])
}

func TestLineMapsCachesLookups(t *testing.T) {
	t.Chdir(t.TempDir())
	maps := newLineMaps("HEAD")

	_, err1 := maps.get(postedOn, "a.go")
	require.Error(t, err1)
	_, err2 := maps.get(postedOn, "a.go")
	assert.Same(t, err1, err2, "the failed lookup is kept, not repeated")
	assert.Len(t, maps.lookups, 1)
}

func TestThreadsToCheckSkips(t *testing.T) {
	threads := []prreview.Thread{openThread("t1", "a.go", 3)}

	tests := []struct {
		name     string
		hist     prreview.History
		checking bool
		head     string
		out      string
	}{
		{
			name:     "threads couldn't be read",
			hist:     prreview.History{ThreadsErr: errors.New("no access"), Threads: threads},
			checking: true, head: "abc",
			out: "bark: warning: no access; fixed threads won't be resolved\n",
		},
		{name: "no open threads", hist: prreview.History{}, checking: true, head: "abc"},
		{
			name: "check is off",
			hist: prreview.History{Threads: threads}, head: "abc",
			out: "bark: fixed threads aren't resolved without Bark's check (--check)\n",
		},
		{name: "head unknown", hist: prreview.History{Threads: threads}, checking: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w bytes.Buffer
			kept, toCheck := threadsToCheck(&w, tt.hist, tt.checking, tt.head, newLineMaps(tt.head))
			assert.Nil(t, kept)
			assert.Nil(t, toCheck)
			assert.Equal(t, tt.out, w.String())
		})
	}
}

func TestThreadCodeAtFailsOutsideARepo(t *testing.T) {
	t.Chdir(t.TempDir())
	code := threadCodeAt(newLineMaps("HEAD"))

	_, _, _, err := code.span(postedOn, "a.go", 1, 2)
	assert.Error(t, err)
	_, err = code.file(postedOn, "a.go")
	assert.Error(t, err)
}

func TestResolveFixed(t *testing.T) {
	var w bytes.Buffer
	assert.Nil(t, resolveFixed(&w, nil, "abc"))
	assert.Empty(t, w.String(), "nothing fixed says nothing")

	failingGHOnPath(t)
	fixed := []prreview.Fixed{{Thread: openThread("t1", "a.go", 3), Reason: "done"}}
	got := resolveFixed(&w, fixed, "abcdef1234")
	assert.Empty(t, got)
	assert.Contains(t, w.String(), "bark: warning: couldn't resolve 1 fixed thread(s):")
}
