package prreview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ionut-t/bark/v2/internal/findings"
)

// Bark resolves the thread of one of its inline comments when its check
// finds the point fixed in the code that changed since. It never resolves
// one because a later review didn't raise the point again: a review of
// only the changes since the last one doesn't see the rest of the code,
// and models don't raise the same points on every run.

// Thread is an open review thread on one of Bark's inline comments.
type Thread struct {
	// ID is the thread's GraphQL node id.
	ID string
	// Comment is the id of the inline comment that starts it.
	Comment int64
	// Point is what the comment raised: its file, title and body, and the
	// lines it covered on Commit, the commit it was posted on.
	Point findings.Finding

	// pull is the pull request's API path.
	pull string
}

// Fixed is a thread Bark's check found fixed, and why.
type Fixed struct {
	Thread Thread
	Reason string
}

// fixedMarker starts every reply Bark posts on a thread it resolves, so
// the reply isn't taken for someone answering the point.
const fixedMarker = "<!-- bark-fixed -->"

// fixedReply reports whether a reply on a thread is Bark's saying it's
// fixed.
func fixedReply(body string) bool {
	return strings.HasPrefix(strings.TrimSpace(body), fixedMarker)
}

// threadsQuery lists a pull request's review threads, a page at a time
// (gh api --paginate fills in $endCursor).
const threadsQuery = `query($owner: String!, $repo: String!, $number: Int!, $endCursor: String) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      reviewThreads(first: 100, after: $endCursor) {
        pageInfo { hasNextPage endCursor }
        nodes { id isResolved comments(first: 1) { nodes { databaseId } } }
      }
    }
  }
}`

// threadsJQ prints each thread as a JSON object on a line of its own: its
// id, whether it's resolved, and the id of the comment that starts it.
const threadsJQ = `.data.repository.pullRequest.reviewThreads.nodes[] | ` +
	`{id, resolved: .isResolved, comment: .comments.nodes[0].databaseId} | @json`

// resolveMutation resolves the review thread with node id $id.
const resolveMutation = `mutation($id: ID!) { resolveReviewThread(input: {threadId: $id}) { thread { isResolved } } }`

// unresolveMutation reopens a thread Bark resolved but couldn't reply on.
const unresolveMutation = `mutation($id: ID!) { unresolveReviewThread(input: {threadId: $id}) { thread { isResolved } } }`

// The Resolved of a point whose thread was resolved on the pull request
// before this review: by someone, or by Bark after saying it's fixed.
const (
	resolvedNote = "resolved on the pull request"
	fixedNote    = "fixed, as Bark's check found"
)

// readThreads reads the review threads of the pull request at pull (an
// API path) and, of Bark's inline comments in inline, marks the findings
// whose thread is resolved and keeps the open threads nobody answered as
// h.Threads. recorded are the findings each of Bark's reviews recorded, by
// review id: a thread covers every line its finding did (see recordedLines).
func (h *History) readThreads(ctx context.Context, run gh, pull string, inline inlineComments, recorded map[int64][]findings.Finding) error {
	parts := strings.Split(pull, "/")
	if len(parts) != 5 {
		return fmt.Errorf("unexpected pull request path %q", pull)
	}
	out, err := run(ctx, nil, "api", "graphql", "--paginate", "-f", "query="+threadsQuery,
		"-f", "owner="+parts[1], "-f", "repo="+parts[2], "-F", "number="+parts[4], "--jq", threadsJQ)
	if err != nil {
		return fmt.Errorf("failed to read review threads: %w", err)
	}
	type thread struct {
		ID       string `json:"id"`
		Resolved bool   `json:"resolved"`
		Comment  int64  `json:"comment"`
	}
	threads := map[int64]thread{}
	for line := range strings.SplitSeq(string(out), "\n") {
		var t thread
		if json.Unmarshal([]byte(line), &t) == nil && t.ID != "" && t.Comment != 0 {
			threads[t.Comment] = t
		}
	}

	for _, c := range inline.bark {
		t, ok := threads[c.ID]
		if !ok {
			continue
		}
		title, body, ok := commentParts(c.Body)
		if !ok {
			continue
		}
		if t.Resolved {
			// Bark's reply saying it's fixed, on a resolved thread: Bark
			// resolved it, and nobody reopened it.
			if inline.fixed[c.ID] {
				h.markResolved(c.Path, title, fixedNote, true)
			} else {
				h.markResolved(c.Path, title, resolvedNote, false)
			}
			continue
		}
		// The commit is read back into git commands: take only a hash.
		if inline.answered[c.ID] || c.Line <= 0 || !commitHash.MatchString(c.Commit) {
			continue
		}
		line, end := recordedLines(c, title, recorded[c.ReviewID])
		h.Threads = append(h.Threads, Thread{
			ID: t.ID, Comment: c.ID, pull: pull,
			Point: findings.Finding{
				File: c.Path, Line: line, EndLine: end, Title: title, Body: body, Commit: c.Commit,
			},
		})
	}
	return nil
}

// recordedLines returns the lines the thread of comment c, titled title,
// covers: those its finding in recorded (its review's) covered, when they
// hold the comment's, or else the comment's own. GitHub takes a comment on
// several lines only within one hunk, so a finding whose lines span two
// hunks is posted on its first line alone; a fix further down its lines
// is still a change to its code.
func recordedLines(c reviewComment, title string, recorded []findings.Finding) (int, int) {
	line, end := c.Line, max(c.EndLine, c.Line)
	for _, f := range recorded {
		fEnd := max(f.EndLine, f.Line)
		if f.File == c.Path && strings.EqualFold(oneLine(f.Title), oneLine(title)) &&
			f.Line <= line && end <= fEnd {
			return f.Line, fEnd
		}
	}
	return line, end
}

// MarkResolved marks the points of h that the threads Bark resolved, as
// of commit head, raised (matched by file and title).
func (h *History) MarkResolved(threads []Thread, head string) {
	for _, t := range threads {
		h.markResolved(t.Point.File, t.Point.Title, "fixed in "+head[:min(shortHash, len(head))], true)
	}
}

func (h *History) markResolved(file, title, how string, fixed bool) {
	for i, f := range h.Findings {
		if f.File == file && f.Resolved == "" && strings.EqualFold(oneLine(f.Title), oneLine(title)) {
			h.Findings[i].Resolved = how
			h.Findings[i].Fixed = fixed
		}
	}
}

// Resolve resolves each fixed thread, then replies on it that it's fixed
// as of commit head, with the check's reason. Resolving first means a
// token that may not resolve threads posts nothing; a thread Bark resolved
// but couldn't reply on is unresolved again, so a resolved thread always
// says why. It returns the threads it resolved; the errors for the rest are
// joined.
func Resolve(ctx context.Context, fixed []Fixed, head string) ([]Thread, error) {
	return resolve(ctx, runGH, fixed, head)
}

func resolve(ctx context.Context, run gh, fixed []Fixed, head string) ([]Thread, error) {
	var resolved []Thread
	var errs []error
	for _, f := range fixed {
		t := f.Thread
		payload, err := json.Marshal(struct {
			Body string `json:"body"`
		}{fixedBody(f.Reason, head)})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if _, err := run(ctx, nil, "api", "graphql", "-f", "query="+resolveMutation, "-f", "id="+t.ID); err != nil {
			errs = append(errs, fmt.Errorf("failed to resolve the thread of %q: %w", t.Point.Title, err))
			continue
		}
		if _, err := run(ctx, payload, "api", "--method", "POST",
			fmt.Sprintf("%s/comments/%d/replies", t.pull, t.Comment), "--input", "-"); err != nil {
			if uerr := unresolve(ctx, run, t.ID); uerr != nil {
				errs = append(errs, fmt.Errorf("failed to reply on the thread of %q: %w; and failed to unresolve it again, so it's resolved without saying why: %w",
					t.Point.Title, err, uerr))
			} else {
				errs = append(errs, fmt.Errorf("failed to reply on the thread of %q, so it was unresolved again: %w", t.Point.Title, err))
			}
			continue
		}
		resolved = append(resolved, t)
	}
	return resolved, errors.Join(errs...)
}

// unresolveTimeout is how long unresolving a thread may take, even once
// the time for resolving has run out.
const unresolveTimeout = 15 * time.Second

// unresolve reopens a thread Bark resolved but couldn't reply on. It has
// its own time limit: the reply may have failed because ctx ran out, and
// the thread mustn't stay resolved without saying why.
func unresolve(ctx context.Context, run gh, id string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), unresolveTimeout)
	defer cancel()
	_, err := run(ctx, nil, "api", "graphql", "-f", "query="+unresolveMutation, "-f", "id="+id)
	return err
}

// maxReason caps the check's reason Bark quotes on a thread it resolves.
const maxReason = 600

// fixedBody is the reply Bark posts on a thread before resolving it.
func fixedBody(reason, head string) string {
	reason = oneLine(reason)
	if len(reason) > maxReason {
		reason = strings.ToValidUTF8(reason[:maxReason], "") + "…"
	}
	return fmt.Sprintf("%s\nFixed in `%s`: %s\n\n_Bark's check found this fixed, so it resolved the thread. Unresolve it if it isn't fixed._",
		fixedMarker, head[:min(shortHash, len(head))], reason)
}
