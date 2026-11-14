package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connect starts a server with opts and returns a client session to it.
func connect(t *testing.T, opts Options) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverT, clientT := mcp.NewInMemoryTransports()

	ss, err := New(opts).Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	return res
}

func text(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) != 1 {
		t.Fatalf("got %d content items, want 1", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content is %T, want text", res.Content[0])
	}
	return tc.Text
}

func TestListTools(t *testing.T) {
	cs := connect(t, Options{})
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, tool := range res.Tools {
		got[tool.Name] = tool.Annotations != nil && tool.Annotations.ReadOnlyHint
	}
	for _, name := range []string{"review_diff", "earlier_findings"} {
		readOnly, ok := got[name]
		if !ok {
			t.Errorf("tool %s not listed", name)
		} else if !readOnly {
			t.Errorf("tool %s isn't marked read-only", name)
		}
	}
}

func TestReviewDiffFormatsLikeTheReviewPrompt(t *testing.T) {
	var gotParams git.ReviewDiffParams
	cs := connect(t, Options{
		Limit: git.DiffLimit{Lines: 50},
		ReviewDiff: func(_ context.Context, p git.ReviewDiffParams) (git.ReviewDiff, error) {
			gotParams = p
			return git.ReviewDiff{
				ContextHeader: "## Branch\nfeat/x\n\n",
				Stat:          " a.go | 1 +",
				Diff:          "+added line\n",
			}, nil
		},
		Context: func(context.Context, git.ReviewDiff) (string, error) {
			return "## Related code\nfunc helper() {}\n\n", nil
		},
	})

	got := text(t, call(t, cs, "review_diff", map[string]any{"source": "staged"}))

	if want := git.WorkingTreeDiff(true).WithLimit(git.DiffLimit{Lines: 50}); gotParams != want {
		t.Errorf("params = %+v, want %+v", gotParams, want)
	}
	for _, want := range []string{"## Branch\nfeat/x", "## Files Changed\n a.go | 1 +", "## Related code\nfunc helper() {}", "**Code to review:**\n+added line"} {
		if !strings.Contains(got, want) {
			t.Errorf("result lacks %q:\n%s", want, got)
		}
	}
}

func TestReviewDiffWithoutContext(t *testing.T) {
	cs := connect(t, Options{
		ReviewDiff: func(context.Context, git.ReviewDiffParams) (git.ReviewDiff, error) {
			return git.ReviewDiff{Diff: "+x\n"}, nil
		},
		Context: func(context.Context, git.ReviewDiff) (string, error) {
			t.Error("context fetched despite without_context")
			return "", nil
		},
	})
	call(t, cs, "review_diff", map[string]any{"without_context": true})
}

func TestReviewDiffKeepsGoingWhenContextFails(t *testing.T) {
	cs := connect(t, Options{
		ReviewDiff: func(context.Context, git.ReviewDiffParams) (git.ReviewDiff, error) {
			return git.ReviewDiff{Diff: "+x\n"}, nil
		},
		Context: func(context.Context, git.ReviewDiff) (string, error) {
			return "", errors.New("no tree-sitter today")
		},
	})
	res := call(t, cs, "review_diff", nil)
	if res.IsError {
		t.Fatalf("got error result: %s", text(t, res))
	}
	if got := text(t, res); !strings.Contains(got, "_Related code unavailable: no tree-sitter today_") {
		t.Errorf("result doesn't say the related code is missing:\n%s", got)
	}
}

// The related code gets its own time, not what the diff left of a shared
// timeout.
func TestReviewDiffContextHasItsOwnTimeout(t *testing.T) {
	const diffTook = 20 * time.Millisecond
	var diffDeadline, relatedDeadline time.Time
	cs := connect(t, Options{
		ReviewDiff: func(ctx context.Context, _ git.ReviewDiffParams) (git.ReviewDiff, error) {
			diffDeadline, _ = ctx.Deadline()
			time.Sleep(diffTook)
			return git.ReviewDiff{Diff: "+x\n"}, nil
		},
		Context: func(ctx context.Context, _ git.ReviewDiff) (string, error) {
			relatedDeadline, _ = ctx.Deadline()
			return "", nil
		},
	})
	call(t, cs, "review_diff", nil)
	if diffDeadline.IsZero() || relatedDeadline.IsZero() {
		t.Fatal("want a deadline on both the diff and the related code")
	}
	if gap := relatedDeadline.Sub(diffDeadline); gap < diffTook {
		t.Errorf("related code deadline is %v after the diff's, want at least %v", gap, diffTook)
	}
}

func TestReviewDiffSources(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want git.ReviewDiffParams
	}{
		{"default", nil, git.WorkingTreeDiff(false)},
		{"changes", map[string]any{"source": "changes"}, git.WorkingTreeDiff(false)},
		{"branch", map[string]any{"source": "branch", "ref": "main"}, git.BranchDiff("main")},
		{"commit", map[string]any{"source": "commit", "ref": "abc123"}, git.CommitDiff("abc123")},
		{"pr", map[string]any{"source": "pr", "ref": "42"}, git.PRDiff("42")},
		{"pr with description", map[string]any{"source": "pr", "ref": "42", "with_pr_description": true}, git.PRDiff("42").WithPRDescription()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got git.ReviewDiffParams
			cs := connect(t, Options{
				ReviewDiff: func(_ context.Context, p git.ReviewDiffParams) (git.ReviewDiff, error) {
					got = p
					return git.ReviewDiff{Diff: "+x\n"}, nil
				},
				Context: func(context.Context, git.ReviewDiff) (string, error) { return "", nil },
			})
			if res := call(t, cs, "review_diff", tt.args); res.IsError {
				t.Fatalf("got error result: %s", text(t, res))
			}
			if got != tt.want {
				t.Errorf("params = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestReviewDiffErrors(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		diff string
		want string
	}{
		{"unknown source", map[string]any{"source": "stash"}, "+x\n", `unknown source "stash"`},
		{"branch without ref", map[string]any{"source": "branch"}, "+x\n", `needs ref`},
		{"ref on staged", map[string]any{"source": "staged", "ref": "main"}, "+x\n", `source "staged" takes no ref`},
		{"ref without source", map[string]any{"ref": "main"}, "+x\n", `source "changes" (the default) takes no ref`},
		{"description without pr", map[string]any{"with_pr_description": true}, "+x\n", `with_pr_description needs source "pr"`},
		{"empty diff", nil, "", "no changes to review"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := connect(t, Options{
				ReviewDiff: func(context.Context, git.ReviewDiffParams) (git.ReviewDiff, error) {
					return git.ReviewDiff{Diff: tt.diff}, nil
				},
				Context: func(context.Context, git.ReviewDiff) (string, error) { return "", nil },
			})
			res := call(t, cs, "review_diff", tt.args)
			if !res.IsError {
				t.Fatal("want an error result")
			}
			if got := text(t, res); !strings.Contains(got, tt.want) {
				t.Errorf("error = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}

func TestEarlierFindings(t *testing.T) {
	var gotPR string
	cs := connect(t, Options{
		Earlier: func(_ context.Context, pr string) ([]findings.Finding, error) {
			gotPR = pr
			return []findings.Finding{{File: "a.go", Line: 3, Severity: findings.High, Title: "nil map write"}}, nil
		},
	})

	res := call(t, cs, "earlier_findings", map[string]any{"pr": "42"})
	if res.IsError {
		t.Fatalf("got error result: %s", text(t, res))
	}
	if gotPR != "42" {
		t.Errorf("pr = %q, want 42", gotPR)
	}

	var out EarlierOutput
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	want := []EarlierFinding{{File: "a.go", Line: 3, Title: "nil map write"}}
	if !reflect.DeepEqual(out.Findings, want) {
		t.Errorf("findings = %+v, want %+v", out.Findings, want)
	}
	if strings.Contains(text(t, res), "severity") {
		t.Errorf("result has a severity, which earlier reviews don't record: %s", text(t, res))
	}
}

func TestEarlierFindingsNoneIsEmptyList(t *testing.T) {
	cs := connect(t, Options{
		Earlier: func(context.Context, string) ([]findings.Finding, error) { return nil, nil },
	})
	res := call(t, cs, "earlier_findings", map[string]any{"pr": "42"})
	if got := text(t, res); !strings.Contains(got, `"findings":[]`) {
		t.Errorf("result = %s, want an empty findings list", got)
	}
}

// The schema requires pr, but an empty one passes it.
func TestEarlierFindingsEmptyPR(t *testing.T) {
	cs := connect(t, Options{
		Earlier: func(context.Context, string) ([]findings.Finding, error) {
			t.Error("Earlier called without a pull request")
			return nil, nil
		},
	})
	res := call(t, cs, "earlier_findings", map[string]any{"pr": ""})
	if !res.IsError || !strings.Contains(text(t, res), "pr is required") {
		t.Errorf("want a tool error, got %+v", res)
	}
}

func TestEarlierFindingsErrors(t *testing.T) {
	cs := connect(t, Options{
		Earlier: func(context.Context, string) ([]findings.Finding, error) {
			return nil, errors.New("gh: not logged in")
		},
	})
	res := call(t, cs, "earlier_findings", map[string]any{"pr": "42"})
	if !res.IsError || !strings.Contains(text(t, res), "not logged in") {
		t.Errorf("want the gh error as a tool error, got %+v", res)
	}
}
