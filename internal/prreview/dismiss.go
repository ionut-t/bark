package prreview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/ionut-t/bark/v2/internal/findings"
)

// A finding Bark posted as an inline comment is dismissed on the pull
// request by a 👎 on that comment from someone with write access, or by a
// reply saying "won't fix" from an owner, member or collaborator. Resolving
// the thread doesn't count: threads are also resolved after a fix.

// reviewCommentsQuery prints each inline comment of a pull request as a
// JSON object on a line of its own. The lines are the first and last ones
// the comment covered on the commit it was posted on, as the review
// recorded it.
const reviewCommentsQuery = `.[] | {id, review_id: .pull_request_review_id, in_reply_to: .in_reply_to_id, path, ` +
	`line: (.original_start_line // .original_line), end_line: .original_line, commit: .original_commit_id, ` +
	`body, user: .user.login, user_type: .user.type, author_association, thumbs_down: .reactions["-1"]} | @json`

// reactionsQuery prints who left each reaction, one JSON object per line.
const reactionsQuery = `.[] | {user: .user.login, user_type: .user.type} | @json`

// barkReview is a review Bark posted, from a trusted reviewer: the commit
// it was posted on (if it's a valid hash) and the findings it recorded.
type barkReview struct {
	id       int64
	commit   string
	findings []findings.Finding
}

type reviewComment struct {
	ID          int64  `json:"id"`
	ReviewID    int64  `json:"review_id"`
	InReplyTo   int64  `json:"in_reply_to"`
	Path        string `json:"path"`
	Line        int    `json:"line"`
	EndLine     int    `json:"end_line"`
	Commit      string `json:"commit"`
	Body        string `json:"body"`
	User        string `json:"user"`
	UserType    string `json:"user_type"`
	Association string `json:"author_association"`
	ThumbsDown  int    `json:"thumbs_down"`
}

// dismissedComment is an inline comment of Bark's that someone dismissed,
// and how.
type dismissedComment struct {
	comment reviewComment
	how     string
}

// inlineComments is what the inline comments of a pull request say about
// the ones Bark posted.
type inlineComments struct {
	// bark are the inline comments of Bark's reviews, without replies.
	bark []reviewComment
	// dismissed are those that were dismissed, by review id.
	dismissed map[int64][]dismissedComment
	// answered are those someone other than a bot replied to, and those
	// Bark said were fixed, by comment id.
	answered map[int64]bool
	// fixed are those Bark said were fixed, by comment id.
	fixed map[int64]bool
}

// readInline reads the inline comments of the reviews in bark (by id):
// which were dismissed, and which were answered. pull is the pull
// request's API path; trusted holds logins known to have write access, and
// gets the ones this finds. Comments whose dismissal it couldn't decide on
// count as not dismissed and the error says why; the rest are still
// returned.
func readInline(ctx context.Context, run gh, pull string, bark map[int64]bool, trusted map[string]bool) (inlineComments, error) {
	var in inlineComments
	out, err := run(ctx, nil, "api", "--paginate", pull+"/comments", "--jq", reviewCommentsQuery)
	if err != nil {
		return in, fmt.Errorf("failed to read review comments: %w", err)
	}

	var comments, barks []reviewComment
	isBark := map[int64]bool{}
	for line := range strings.SplitSeq(string(out), "\n") {
		var c reviewComment
		if json.Unmarshal([]byte(line), &c) != nil {
			continue
		}
		comments = append(comments, c)
		if c.UserType != "Bot" && trustedReviewer(c.UserType, c.Association) {
			trusted[c.User] = true
		}
		if bark[c.ReviewID] && c.InReplyTo == 0 {
			barks = append(barks, c)
			isBark[c.ID] = true
		}
	}

	how := map[int64]string{}
	in.answered = map[int64]bool{}
	in.fixed = map[int64]bool{}
	for _, c := range comments {
		if !isBark[c.InReplyTo] {
			continue
		}
		// Bark's own reply saying the point is fixed is posted as whoever
		// runs it, which may be a member: it's never a dismissal.
		if fixedReply(c.Body) {
			in.answered[c.InReplyTo] = true
			in.fixed[c.InReplyTo] = true
			continue
		}
		if c.UserType != "Bot" {
			in.answered[c.InReplyTo] = true
		}
		if how[c.InReplyTo] == "" && c.UserType != "Bot" &&
			trustedReviewer(c.UserType, c.Association) && wontFix(c.Body) {
			how[c.InReplyTo] = fmt.Sprintf("%q from %s", "won't fix", c.User)
		}
	}

	repo := pull[:strings.LastIndex(pull, "/pulls/")]
	var errs []error
	canWrite := func(login string) bool {
		if ok, known := trusted[login]; known {
			return ok
		}
		ok, err := writeAccess(ctx, run, repo, login)
		if err != nil {
			errs = append(errs, err)
			return false
		}
		trusted[login] = ok
		return ok
	}
	for _, c := range barks {
		if c.ThumbsDown == 0 || how[c.ID] != "" {
			continue
		}
		out, err := run(ctx, nil, "api", "--paginate", fmt.Sprintf("%s/pulls/comments/%d/reactions?content=-1", repo, c.ID),
			"--jq", reactionsQuery)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to read reactions to comment %d: %w", c.ID, err))
			continue
		}
		for line := range strings.SplitSeq(string(out), "\n") {
			var r struct {
				User     string `json:"user"`
				UserType string `json:"user_type"`
			}
			if json.Unmarshal([]byte(line), &r) != nil || r.User == "" || r.UserType == "Bot" {
				continue
			}
			if canWrite(r.User) {
				how[c.ID] = "👎 from " + r.User
				break
			}
		}
	}

	in.bark = barks
	in.dismissed = map[int64][]dismissedComment{}
	for _, c := range barks {
		if how[c.ID] != "" {
			in.dismissed[c.ReviewID] = append(in.dismissed[c.ReviewID], dismissedComment{comment: c, how: how[c.ID]})
			in.answered[c.ID] = true
		}
	}
	return in, errors.Join(errs...)
}

// writeAccess reports whether login has write access to repo (an API path
// such as repos/owner/repo). A user who isn't a collaborator has none.
func writeAccess(ctx context.Context, run gh, repo, login string) (bool, error) {
	out, err := run(ctx, nil, "api", fmt.Sprintf("%s/collaborators/%s/permission", repo, url.PathEscape(login)), "--jq", ".permission")
	if err != nil {
		if strings.Contains(err.Error(), "HTTP 404") {
			return false, nil
		}
		return false, fmt.Errorf("failed to look up %s's permission: %w", login, err)
	}
	switch strings.TrimSpace(string(out)) {
	case "admin", "maintain", "write":
		return true, nil
	}
	return false, nil
}

var wontFixRe = regexp.MustCompile(`(?i)\bwon['’]?t[ -]?fix\b`)

// wontFix reports whether a reply says the finding won't be fixed.
func wontFix(body string) bool {
	return wontFixRe.MatchString(body)
}

// commentHeading matches the first line of an inline comment Bark posts
// (see commentBody): "**[SEVERITY · category] Title**".
var commentHeading = regexp.MustCompile(`^\*\*\[[^\]]*\] (.+)\*\*$`)

// commentFinding returns the title and the body's first sentence of the
// finding an inline comment of Bark's posted.
func commentFinding(body string) (title, first string, ok bool) {
	title, para, ok := commentParts(body)
	return title, findings.FirstSentence(para), ok
}

// commentParts returns the title and the first paragraph of the body of
// the finding an inline comment of Bark's posted (see commentBody): the
// note that Bark's check didn't check it is skipped, and a suggestion or
// remark isn't the body.
func commentParts(body string) (title, para string, ok bool) {
	head, rest, _ := strings.Cut(strings.TrimSpace(body), "\n")
	m := commentHeading.FindStringSubmatch(strings.TrimSpace(head))
	if m == nil {
		return "", "", false
	}
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, findings.UncheckedNote) {
		_, rest, _ = strings.Cut(rest, "\n\n")
	}
	para, _, _ = strings.Cut(strings.TrimSpace(rest), "\n\n")
	if strings.HasPrefix(para, "```") || strings.HasPrefix(para, "> _") {
		para = ""
	}
	return m[1], strings.TrimSpace(para), true
}

// markDismissed marks the findings a review recorded that the comments in
// dismissed posted, matched by file and title. A dismissed comment whose
// finding the review didn't record is added as a finding of its own, with
// commit, the one the review was posted on.
func markDismissed(recorded []findings.Finding, dismissed []dismissedComment, commit string) []findings.Finding {
	for _, d := range dismissed {
		title, body, ok := commentFinding(d.comment.Body)
		if !ok {
			continue
		}
		matched := false
		for i, f := range recorded {
			if f.File == d.comment.Path && strings.EqualFold(oneLine(f.Title), oneLine(title)) {
				matched = true
				if recorded[i].Dismissed == "" {
					recorded[i].Dismissed = d.how
				}
			}
		}
		if !matched {
			recorded = append(recorded, findings.Finding{
				File: d.comment.Path, Line: d.comment.Line, Title: title, Body: body,
				Dismissed: d.how, Commit: commit,
			})
		}
	}
	return recorded
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
