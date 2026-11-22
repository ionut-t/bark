package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Review is a review posted on a pull request.
type Review struct {
	ID       int64  `json:"id"`
	CommitID string `json:"commit_id"`
	// State is APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED or
	// PENDING.
	State    string `json:"state"`
	Body     string `json:"body"`
	UserType string `json:"user_type"`
	// Association is the poster's author_association with the repository,
	// such as OWNER, MEMBER, COLLABORATOR or CONTRIBUTOR.
	Association string `json:"author_association"`
}

// reviewsQuery prints each review as a JSON object on a line of its own.
const reviewsQuery = `.[] | {id, commit_id, state, body, user_type: .user.type, author_association} | @json`

// Reviews lists the reviews of pull, oldest first.
func (c *Client) Reviews(ctx context.Context, pull Pull) ([]Review, error) {
	out, err := c.run(ctx, nil, "api", "--paginate", pull.path()+"/reviews", "--jq", reviewsQuery)
	if err != nil {
		return nil, err
	}
	return jsonLines[Review](out), nil
}

// PostReview posts review, the request body of GitHub's "create a review
// for a pull request" endpoint, on pull, and returns its URL.
func (c *Client) PostReview(ctx context.Context, pull Pull, review any) (string, error) {
	payload, err := json.Marshal(review)
	if err != nil {
		return "", err
	}
	out, err := c.run(ctx, payload, "api", "--method", "POST", pull.path()+"/reviews", "--input", "-")
	if err != nil {
		return "", err
	}
	var created struct {
		HTMLURL string `json:"html_url"`
	}
	_ = json.Unmarshal(out, &created)
	return created.HTMLURL, nil
}

// DismissReview dismisses review id of pull, saying why in message.
func (c *Client) DismissReview(ctx context.Context, pull Pull, id int64, message string) error {
	payload, err := json.Marshal(struct {
		Message string `json:"message"`
		Event   string `json:"event"`
	}{message, "DISMISS"})
	if err != nil {
		return err
	}
	_, err = c.run(ctx, payload, "api", "--method", "PUT",
		fmt.Sprintf("%s/reviews/%d/dismissals", pull.path(), id), "--input", "-")
	return err
}

// ReviewComment is an inline comment of a review.
type ReviewComment struct {
	ID       int64 `json:"id"`
	ReviewID int64 `json:"review_id"`
	// InReplyTo is the comment it replies to, or 0.
	InReplyTo int64  `json:"in_reply_to"`
	Path      string `json:"path"`
	// Line and EndLine are the first and last lines the comment covered
	// on Commit, the commit it was posted on.
	Line        int    `json:"line"`
	EndLine     int    `json:"end_line"`
	Commit      string `json:"commit"`
	Body        string `json:"body"`
	User        string `json:"user"`
	UserType    string `json:"user_type"`
	Association string `json:"author_association"`
	// ThumbsDown is how many 👎 it has.
	ThumbsDown int `json:"thumbs_down"`
}

// reviewCommentsQuery prints each inline comment as a JSON object on a
// line of its own. The lines are those the comment covered on the commit
// it was posted on, as the review recorded it.
const reviewCommentsQuery = `.[] | {id, review_id: .pull_request_review_id, in_reply_to: .in_reply_to_id, path, ` +
	`line: (.original_start_line // .original_line), end_line: .original_line, commit: .original_commit_id, ` +
	`body, user: .user.login, user_type: .user.type, author_association, thumbs_down: .reactions["-1"]} | @json`

// ReviewComments lists the inline comments of pull's reviews, replies
// included.
func (c *Client) ReviewComments(ctx context.Context, pull Pull) ([]ReviewComment, error) {
	out, err := c.run(ctx, nil, "api", "--paginate", pull.path()+"/comments", "--jq", reviewCommentsQuery)
	if err != nil {
		return nil, err
	}
	return jsonLines[ReviewComment](out), nil
}

// Reply replies body to the inline comment id of pull.
func (c *Client) Reply(ctx context.Context, pull Pull, id int64, body string) error {
	payload, err := json.Marshal(struct {
		Body string `json:"body"`
	}{body})
	if err != nil {
		return err
	}
	_, err = c.run(ctx, payload, "api", "--method", "POST",
		fmt.Sprintf("%s/comments/%d/replies", pull.path(), id), "--input", "-")
	return err
}

// Reactor is someone who reacted to a comment.
type Reactor struct {
	User     string `json:"user"`
	UserType string `json:"user_type"`
}

// reactionsQuery prints who left each reaction, one JSON object per line.
const reactionsQuery = `.[] | {user: .user.login, user_type: .user.type} | @json`

// ThumbsDown lists who left a 👎 on the inline comment id in repo.
func (c *Client) ThumbsDown(ctx context.Context, repo Repo, id int64) ([]Reactor, error) {
	out, err := c.run(ctx, nil, "api", "--paginate",
		fmt.Sprintf("%s/pulls/comments/%d/reactions?content=-1", repo.path(), id), "--jq", reactionsQuery)
	if err != nil {
		return nil, err
	}
	return jsonLines[Reactor](out), nil
}

// CanWrite reports whether login has write access to repo. A user who
// isn't a collaborator has none.
func (c *Client) CanWrite(ctx context.Context, repo Repo, login string) (bool, error) {
	out, err := c.run(ctx, nil, "api", fmt.Sprintf("%s/collaborators/%s/permission", repo.path(), url.PathEscape(login)),
		"--jq", ".permission")
	if err != nil {
		if strings.Contains(err.Error(), "HTTP 404") {
			return false, nil
		}
		return false, err
	}
	switch strings.TrimSpace(string(out)) {
	case "admin", "maintain", "write":
		return true, nil
	}
	return false, nil
}

// ReviewThread is a thread of inline comments.
type ReviewThread struct {
	// ID is its GraphQL node id.
	ID       string `json:"id"`
	Resolved bool   `json:"resolved"`
	// Comment is the id of the comment that starts it.
	Comment int64 `json:"comment"`
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

// ReviewThreads lists the review threads of pull.
func (c *Client) ReviewThreads(ctx context.Context, pull Pull) ([]ReviewThread, error) {
	out, err := c.run(ctx, nil, "api", "graphql", "--paginate", "-f", "query="+threadsQuery,
		"-f", "owner="+pull.Owner, "-f", "repo="+pull.Name, "-F", "number="+strconv.Itoa(pull.Number), "--jq", threadsJQ)
	if err != nil {
		return nil, err
	}
	return jsonLines[ReviewThread](out), nil
}

// resolveMutation resolves the review thread with node id $id.
const resolveMutation = `mutation($id: ID!) { resolveReviewThread(input: {threadId: $id}) { thread { isResolved } } }`

// unresolveMutation reopens the review thread with node id $id.
const unresolveMutation = `mutation($id: ID!) { unresolveReviewThread(input: {threadId: $id}) { thread { isResolved } } }`

// ResolveThread resolves the review thread with node id id.
func (c *Client) ResolveThread(ctx context.Context, id string) error {
	_, err := c.run(ctx, nil, "api", "graphql", "-f", "query="+resolveMutation, "-f", "id="+id)
	return err
}

// UnresolveThread reopens the review thread with node id id.
func (c *Client) UnresolveThread(ctx context.Context, id string) error {
	_, err := c.run(ctx, nil, "api", "graphql", "-f", "query="+unresolveMutation, "-f", "id="+id)
	return err
}

// jsonLines decodes out, one JSON value per line, skipping the lines that
// aren't one.
func jsonLines[T any](out []byte) []T {
	var items []T
	for line := range strings.SplitSeq(string(out), "\n") {
		var v T
		if json.Unmarshal([]byte(line), &v) == nil {
			items = append(items, v)
		}
	}
	return items
}
