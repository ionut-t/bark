package github

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var pull = Pull{Owner: "o", Name: "r", Number: 7}

// fakeRun records each gh call and answers it with out and err.
type fakeRun struct {
	out   string
	err   error
	args  [][]string
	stdin [][]byte
}

func (f *fakeRun) run(_ context.Context, stdin []byte, args ...string) ([]byte, error) {
	f.args = append(f.args, args)
	f.stdin = append(f.stdin, stdin)
	return []byte(f.out), f.err
}

func client(out string, err error) (*Client, *fakeRun) {
	f := &fakeRun{out: out, err: err}
	return &Client{run: f.run}, f
}

func TestParsePullURL(t *testing.T) {
	t.Parallel()

	got, err := ParsePullURL("https://github.com/ionut-t/bark/pull/12")
	require.NoError(t, err)
	assert.Equal(t, Pull{Owner: "ionut-t", Name: "bark", Number: 12}, got)
	assert.Equal(t, "repos/ionut-t/bark/pulls/12", got.path())

	for _, u := range []string{"https://github.com/ionut-t/bark", "https://github.com/ionut-t/bark/issues/12",
		"https://github.com/ionut-t/bark/pull/x", "https://github.com//bark/pull/12", ":"} {
		_, err := ParsePullURL(u)
		assert.Error(t, err, u)
	}
}

func TestPullRef(t *testing.T) {
	t.Parallel()

	c, f := client(`{"url":"https://github.com/o/r/pull/7","headRefOid":"abc123"}`, nil)
	got, err := c.PullRef(t.Context(), "feature")
	require.NoError(t, err)
	assert.Equal(t, PullRef{Pull: pull, URL: "https://github.com/o/r/pull/7", HeadSHA: "abc123"}, got)
	assert.Equal(t, [][]string{{"pr", "view", "feature", "--json", "url,headRefOid"}}, f.args)

	c, _ = client(`{"url":"https://github.com/o/r"}`, nil)
	_, err = c.PullRef(t.Context(), "7")
	assert.ErrorContains(t, err, "unexpected pull request URL")
}

func TestPullRequest(t *testing.T) {
	t.Parallel()

	c, f := client(`{"number":7,"title":"T","body":"B","headRefOid":"h","baseRefOid":"b","changedFiles":3,
		"files":[{"path":"a.go","additions":2,"deletions":1}],
		"commits":[{"messageHeadline":"feat: x","messageBody":"why"}]}`, nil)
	got, err := c.PullRequest(t.Context(), "7")
	require.NoError(t, err)
	assert.Equal(t, PullRequest{
		Number: 7, Title: "T", Body: "B", HeadSHA: "h", BaseSHA: "b", ChangedFiles: 3,
		Files:   []File{{Path: "a.go", Additions: 2, Deletions: 1}},
		Commits: []Commit{{Headline: "feat: x", Body: "why"}},
	}, got)
	assert.Equal(t, [][]string{{"pr", "view", "7", "--json", "commits,title,number,body,headRefOid,baseRefOid,files,changedFiles"}}, f.args)
}

func TestPullRequestDiff(t *testing.T) {
	t.Parallel()

	c, f := client("diff --git a/a b/a\n", nil)
	got, err := c.PullRequestDiff(t.Context(), "7")
	require.NoError(t, err)
	assert.Equal(t, "diff --git a/a b/a\n", got)
	assert.Equal(t, [][]string{{"pr", "diff", "7"}}, f.args)
}

func TestFileContent(t *testing.T) {
	t.Parallel()

	c, f := client("package a\n", nil)
	got, err := c.FileContent(t.Context(), "abc123", "dir/a b#.go")
	require.NoError(t, err)
	assert.Equal(t, "package a\n", string(got))
	assert.Equal(t, [][]string{{"api", "-H", "Accept: application/vnd.github.raw",
		"repos/{owner}/{repo}/contents/dir/a%20b%23.go?ref=abc123"}}, f.args)
}

func TestReviews(t *testing.T) {
	t.Parallel()

	c, f := client(`{"id":1,"commit_id":"abc","state":"APPROVED","body":"b","user_type":"Bot","author_association":"NONE"}`+
		"\nnot json\n"+`{"id":2,"body":"c","user_type":"User","author_association":"MEMBER"}`+"\n", nil)
	got, err := c.Reviews(t.Context(), pull)
	require.NoError(t, err)
	assert.Equal(t, []Review{
		{ID: 1, CommitID: "abc", State: "APPROVED", Body: "b", UserType: "Bot", Association: "NONE"},
		{ID: 2, Body: "c", UserType: "User", Association: "MEMBER"},
	}, got, "lines that aren't JSON are skipped")
	assert.Equal(t, [][]string{{"api", "--paginate", "repos/o/r/pulls/7/reviews", "--jq", reviewsQuery}}, f.args)
}

func TestPostReview(t *testing.T) {
	t.Parallel()

	c, f := client(`{"html_url":"https://github.com/o/r/pull/7#pullrequestreview-1"}`, nil)
	got, err := c.PostReview(t.Context(), pull, map[string]string{"event": "COMMENT"})
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/o/r/pull/7#pullrequestreview-1", got)
	assert.Equal(t, [][]string{{"api", "--method", "POST", "repos/o/r/pulls/7/reviews", "--input", "-"}}, f.args)
	assert.JSONEq(t, `{"event":"COMMENT"}`, string(f.stdin[0]))

	c, _ = client("", errors.New("HTTP 422"))
	_, err = c.PostReview(t.Context(), pull, struct{}{})
	assert.ErrorContains(t, err, "HTTP 422")
}

func TestDismissReview(t *testing.T) {
	t.Parallel()

	c, f := client("{}", nil)
	require.NoError(t, c.DismissReview(t.Context(), pull, 42, "No longer."))
	assert.Equal(t, [][]string{{"api", "--method", "PUT", "repos/o/r/pulls/7/reviews/42/dismissals", "--input", "-"}}, f.args)
	assert.JSONEq(t, `{"message":"No longer.","event":"DISMISS"}`, string(f.stdin[0]))
}

func TestReviewComments(t *testing.T) {
	t.Parallel()

	line, _ := json.Marshal(map[string]any{"id": 101, "review_id": 1, "in_reply_to": 0, "path": "a.go", "line": 2,
		"end_line": 3, "commit": "abc", "body": "b", "user": "u", "user_type": "Bot", "author_association": "NONE",
		"thumbs_down": 1})
	c, f := client(string(line)+"\n", nil)
	got, err := c.ReviewComments(t.Context(), pull)
	require.NoError(t, err)
	assert.Equal(t, []ReviewComment{{ID: 101, ReviewID: 1, Path: "a.go", Line: 2, EndLine: 3, Commit: "abc", Body: "b",
		User: "u", UserType: "Bot", Association: "NONE", ThumbsDown: 1}}, got)
	assert.Equal(t, [][]string{{"api", "--paginate", "repos/o/r/pulls/7/comments", "--jq", reviewCommentsQuery}}, f.args)
}

func TestReply(t *testing.T) {
	t.Parallel()

	c, f := client("{}", nil)
	require.NoError(t, c.Reply(t.Context(), pull, 101, "Fixed."))
	assert.Equal(t, [][]string{{"api", "--method", "POST", "repos/o/r/pulls/7/comments/101/replies", "--input", "-"}}, f.args)
	assert.JSONEq(t, `{"body":"Fixed."}`, string(f.stdin[0]))
}

func TestThumbsDown(t *testing.T) {
	t.Parallel()

	c, f := client(`{"user":"owner","user_type":"User"}`+"\n"+`{"user":"dependabot[bot]","user_type":"Bot"}`+"\n", nil)
	got, err := c.ThumbsDown(t.Context(), pull.Repo, 101)
	require.NoError(t, err)
	assert.Equal(t, []Reactor{{User: "owner", UserType: "User"}, {User: "dependabot[bot]", UserType: "Bot"}}, got)
	assert.Equal(t, [][]string{{"api", "--paginate", "repos/o/r/pulls/comments/101/reactions?content=-1", "--jq", reactionsQuery}}, f.args)
}

func TestCanWrite(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		out  string
		err  error
		want bool
	}{
		{out: "admin\n", want: true},
		{out: "maintain\n", want: true},
		{out: "write\n", want: true},
		{out: "triage\n"},
		{out: "read\n"},
		{err: errors.New("exit status 1: gh: Not Found (HTTP 404)")},
	} {
		c, f := client(tc.out, tc.err)
		got, err := c.CanWrite(t.Context(), pull.Repo, "some one")
		require.NoError(t, err, "a 404 only means not a collaborator")
		assert.Equal(t, tc.want, got, tc.out)
		assert.Equal(t, [][]string{{"api", "repos/o/r/collaborators/some%20one/permission", "--jq", ".permission"}}, f.args)
	}

	c, _ := client("", errors.New("HTTP 502"))
	_, err := c.CanWrite(t.Context(), pull.Repo, "x")
	assert.ErrorContains(t, err, "HTTP 502")
}

func TestReviewThreads(t *testing.T) {
	t.Parallel()

	c, f := client(`{"id":"T1","resolved":true,"comment":101}`+"\n"+`{"id":"T2","resolved":false,"comment":102}`+"\n", nil)
	got, err := c.ReviewThreads(t.Context(), pull)
	require.NoError(t, err)
	assert.Equal(t, []ReviewThread{{ID: "T1", Resolved: true, Comment: 101}, {ID: "T2", Comment: 102}}, got)
	assert.Equal(t, [][]string{{"api", "graphql", "--paginate", "-f", "query=" + threadsQuery,
		"-f", "owner=o", "-f", "repo=r", "-F", "number=7", "--jq", threadsJQ}}, f.args)
}

func TestResolveThread(t *testing.T) {
	t.Parallel()

	c, f := client(`{"data":{}}`, nil)
	require.NoError(t, c.ResolveThread(t.Context(), "T1"))
	require.NoError(t, c.UnresolveThread(t.Context(), "T1"))
	assert.Equal(t, [][]string{
		{"api", "graphql", "-f", "query=" + resolveMutation, "-f", "id=T1"},
		{"api", "graphql", "-f", "query=" + unresolveMutation, "-f", "id=T1"},
	}, f.args)
}

// gh's own errors carry what it printed: the status on stderr, and the
// response body on stdout.
func TestRunGH(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)

	_, err := runGH(t.Context(), nil, "pr", "view")
	require.ErrorIs(t, err, ErrNotInstalled)

	script := "#!/bin/sh\nif [ \"$1\" = ok ]; then /bin/cat; exit; fi\necho '{\"message\":\"Not Found\"}'\necho 'gh: Not Found (HTTP 404)' >&2\nexit 1\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755))

	out, err := runGH(t.Context(), []byte("input"), "ok")
	require.NoError(t, err)
	assert.Equal(t, "input", string(out), "stdin reaches gh")

	_, err = runGH(t.Context(), nil, "api", "x")
	assert.ErrorContains(t, err, "exit status 1: gh: Not Found (HTTP 404)")
	assert.ErrorContains(t, err, `{"message":"Not Found"}`)
}
