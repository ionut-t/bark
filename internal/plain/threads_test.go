package plain

import (
	"errors"
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/check"
	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/prreview"
	"github.com/stretchr/testify/assert"
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
