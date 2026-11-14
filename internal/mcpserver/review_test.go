package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/paths"
	"github.com/ionut-t/bark/v2/internal/plain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// reviewServer returns a session to a server whose review runs fake.
func reviewServer(t *testing.T, reviewerNames []string, fake func(context.Context, plain.ReviewOptions) (plain.ReviewResult, error)) *mcp.ClientSession {
	t.Helper()
	return connect(t, Options{
		Config:    config.New("", config.Data{}),
		Paths:     paths.Paths{Config: "/config"},
		Reviewers: reviewerNames,
		Review:    fake,
	})
}

func reviewOutput(t *testing.T, res *mcp.CallToolResult) ReviewOutput {
	t.Helper()
	if res.IsError {
		t.Fatalf("got error result: %s", text(t, res))
	}
	var out ReviewOutput
	raw, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestReviewToolOnlyWithConfig(t *testing.T) {
	for _, withConfig := range []bool{false, true} {
		opts := Options{}
		if withConfig {
			opts.Config = config.New("", config.Data{})
		}
		res, err := connect(t, opts).ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		listed := false
		for _, tool := range res.Tools {
			listed = listed || tool.Name == "review"
		}
		if listed != withConfig {
			t.Errorf("with config %v: review listed = %v", withConfig, listed)
		}
	}
}

func TestReviewListsSavedReviewers(t *testing.T) {
	cs := reviewServer(t, []string{"Linus", "Yoda"}, nil)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if tool.Name == "review" && !strings.Contains(tool.Description, "Saved reviewers: Linus, Yoda.") {
			t.Errorf("description doesn't list the reviewers: %s", tool.Description)
		}
	}
}

func TestReviewOptions(t *testing.T) {
	no := false
	tests := []struct {
		name string
		args map[string]any
		want func(plain.ReviewOptions) string // "" if fine
	}{
		{"defaults", map[string]any{"reviewer": "Linus"}, func(o plain.ReviewOptions) string {
			if o.ReviewerName != "Linus" || o.Staged || o.Branch != "" || o.Hash != "" || o.PR != "" || o.Check != nil {
				return fmt.Sprintf("%+v", o)
			}
			return ""
		}},
		{"staged", map[string]any{"source": "staged"}, func(o plain.ReviewOptions) string {
			if !o.Staged {
				return "not staged"
			}
			return ""
		}},
		{"branch", map[string]any{"source": "branch", "ref": "main"}, func(o plain.ReviewOptions) string {
			if o.Branch != "main" {
				return "branch = " + o.Branch
			}
			return ""
		}},
		{"commit", map[string]any{"source": "commit", "ref": "abc"}, func(o plain.ReviewOptions) string {
			if o.Hash != "abc" {
				return "hash = " + o.Hash
			}
			return ""
		}},
		{"pr", map[string]any{"source": "pr", "ref": "42", "with_pr_description": true}, func(o plain.ReviewOptions) string {
			if o.PR != "42" || !o.WithPRDescription {
				return fmt.Sprintf("pr = %q, description = %v", o.PR, o.WithPRDescription)
			}
			return ""
		}},
		{"instructions and check", map[string]any{"instructions": []string{"security", "be brief"}, "check": no}, func(o plain.ReviewOptions) string {
			if strings.Join(o.Instructions, "|") != "security|be brief" || o.Check == nil || *o.Check {
				return fmt.Sprintf("instructions = %q, check = %v", o.Instructions, o.Check)
			}
			return ""
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got plain.ReviewOptions
			cs := reviewServer(t, nil, func(_ context.Context, o plain.ReviewOptions) (plain.ReviewResult, error) {
				got = o
				return plain.ReviewResult{}, nil
			})
			reviewOutput(t, call(t, cs, "review", tt.args))

			// Always: structured, nothing streamed, posted or written to stdout.
			if !got.Structured || got.Stream || got.Comment || got.Out == nil || got.Log == nil {
				t.Errorf("structured %v, stream %v, comment %v, out %v, log %v", got.Structured, got.Stream, got.Comment, got.Out, got.Log)
			}
			if got.Paths.Config != "/config" || got.Config == nil {
				t.Errorf("paths %+v, config %v", got.Paths, got.Config)
			}
			if msg := tt.want(got); msg != "" {
				t.Error(msg)
			}
		})
	}
}

func TestReviewResult(t *testing.T) {
	cs := reviewServer(t, nil, func(_ context.Context, o plain.ReviewOptions) (plain.ReviewResult, error) {
		_, _ = fmt.Fprintln(o.Log, "bark: the check kept 1 of 2 finding(s)")
		_, _ = fmt.Fprintln(o.Log, "bark: cost $0.01 (2 calls)")
		_, _ = fmt.Fprintln(o.Out, "# Markdown the agent doesn't need")
		return plain.ReviewResult{Checked: 2, Events: []findings.Event{
			{Intro: "Hmm."},
			{Finding: &findings.Finding{File: "a.go", Line: 3, EndLine: 4, Severity: findings.High, Category: "bug", Title: "Nil map", Body: "m is nil.", Original: "m[k] = v", Suggestion: "m = map[string]int{}\nm[k] = v"}},
			{Finding: &findings.Finding{File: "b.go", Line: 9, Severity: findings.Low, Title: "Naming", Doubt: "It's a preference."}},
			{Text: "stray line"},
			{Verdict: "Fix the map."},
		}}, nil
	})

	out := reviewOutput(t, call(t, cs, "review", map[string]any{"reviewer": "Linus"}))

	if out.Intro != "Hmm." || out.Verdict != "Fix the map." || out.Text != "stray line" || out.Checked != 2 {
		t.Errorf("intro %q, verdict %q, text %q, checked %d", out.Intro, out.Verdict, out.Text, out.Checked)
	}
	want := []ReviewFinding{
		{File: "a.go", Line: 3, EndLine: 4, Severity: "high", Category: "bug", Title: "Nil map", Body: "m is nil.", Original: "m[k] = v", Suggestion: "m = map[string]int{}\nm[k] = v"},
		{File: "b.go", Line: 9, Severity: "low", Title: "Naming", Doubt: "It's a preference."},
	}
	if len(out.Findings) != len(want) {
		t.Fatalf("findings = %+v", out.Findings)
	}
	for i := range want {
		if out.Findings[i] != want[i] {
			t.Errorf("finding %d = %+v, want %+v", i, out.Findings[i], want[i])
		}
	}
	if strings.Join(out.Notes, "|") != "the check kept 1 of 2 finding(s)|cost $0.01 (2 calls)" {
		t.Errorf("notes = %q", out.Notes)
	}
}

func TestReviewNoFindingsIsEmptyList(t *testing.T) {
	cs := reviewServer(t, nil, func(context.Context, plain.ReviewOptions) (plain.ReviewResult, error) {
		return plain.ReviewResult{Events: []findings.Event{{Verdict: "Fine."}}}, nil
	})
	res := call(t, cs, "review", map[string]any{"reviewer": "Linus"})
	if got := text(t, res); !strings.Contains(got, `"findings":[]`) {
		t.Errorf("result = %s, want an empty findings list", got)
	}
}

func TestReviewErrors(t *testing.T) {
	tests := []struct {
		name      string
		reviewers []string
		args      map[string]any
		err       error
		log       string
		want      string
	}{
		{"bad source", nil, map[string]any{"source": "stash"}, nil, "", `unknown source "stash"`},
		{"no reviewer, saved ones", []string{"Linus", "Yoda"}, nil, plain.ErrNoReviewer, "", "pass one of Linus, Yoda"},
		{"no reviewer, none saved", nil, nil, plain.ErrNoReviewer, "", "add .bark/reviewer.md"},
		{"failure keeps Bark's notes", nil, map[string]any{"reviewer": "Linus"}, errors.New("error during review: 429"), "bark: debug log: /tmp/x.log\n", "error during review: 429 (debug log: /tmp/x.log)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			cs := reviewServer(t, tt.reviewers, func(_ context.Context, o plain.ReviewOptions) (plain.ReviewResult, error) {
				called = true
				_, _ = fmt.Fprint(o.Log, tt.log)
				return plain.ReviewResult{}, tt.err
			})
			res := call(t, cs, "review", tt.args)
			if !res.IsError {
				t.Fatal("want an error result")
			}
			if got := text(t, res); !strings.Contains(got, tt.want) {
				t.Errorf("error = %q, want it to contain %q", got, tt.want)
			}
			if tt.err == nil && called {
				t.Error("review ran despite bad arguments")
			}
		})
	}
}
