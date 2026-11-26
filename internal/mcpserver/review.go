package mcpserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/plain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ReviewInput is review's arguments.
type ReviewInput struct {
	Source            string   `json:"source,omitempty" jsonschema:"what to review: changes (all uncommitted changes, the default), staged, branch (the current branch against ref), commit (the commit ref) or pr (GitHub pull request ref)"`
	Ref               string   `json:"ref,omitempty" jsonschema:"the base branch for source branch, a commit hash for commit, a pull request number, URL or branch for pr"`
	WithPRDescription bool     `json:"with_pr_description,omitempty" jsonschema:"give the reviewer the pull request's description (source pr only)"`
	Reviewer          string   `json:"reviewer,omitempty" jsonschema:"a saved reviewer's name or a reviewer file's path; defaults to the repository's .bark/reviewer.md"`
	Instructions      []string `json:"instructions,omitempty" jsonschema:"review instructions: saved instructions' names, file paths or raw text; defaults to the repository's .bark/review.md"`
	Check             *bool    `json:"check,omitempty" jsonschema:"run Bark's check, a second model call that reads the full files and sets aside findings it doubts; defaults to Bark's check_findings setting"`
}

// ReviewOutput is review's result.
type ReviewOutput struct {
	// Intro and Verdict are the reviewer persona's words.
	Intro    string          `json:"intro,omitempty"`
	Findings []ReviewFinding `json:"findings"`
	Verdict  string          `json:"verdict,omitempty"`
	// Text is model output that wasn't in the findings format.
	Text string `json:"text,omitempty"`
	// Checked is how many findings Bark's check looked at; 0 if it didn't
	// run.
	Checked int `json:"checked"`
	// Notes are Bark's own messages: the check's counts, costs, the debug
	// log's path, warnings.
	Notes []string `json:"notes,omitempty"`
}

// ReviewFinding is a finding with what Bark's check made of it.
type ReviewFinding struct {
	File       string `json:"file"`
	Line       int    `json:"line,omitempty"`
	EndLine    int    `json:"end_line,omitempty"`
	Severity   string `json:"severity"`
	Category   string `json:"category,omitempty"`
	Title      string `json:"title"`
	Body       string `json:"body,omitempty"`
	Original   string `json:"original,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
	// Doubt is why Bark's check doubts the finding; empty if it was kept or
	// not checked.
	Doubt string `json:"doubt,omitempty"`
	// RaisedBefore is the point an earlier review of the pull request raised
	// that this finding repeats.
	RaisedBefore string `json:"raised_before,omitempty"`
	// Dismissed is the point someone dismissed on the pull request that
	// this finding repeats.
	Dismissed string `json:"dismissed,omitempty"`
	// Unchecked is why Bark's check didn't check the finding. Unlike the
	// three above, it isn't set aside.
	Unchecked string `json:"unchecked,omitempty"`
	// Repaired means the model's line wasn't valid JSON and Bark fixed it;
	// its fields may be cut short.
	Repaired bool `json:"repaired,omitempty"`
}

func addReviewTool(server *mcp.Server, opts Options) {
	desc := "Runs Bark's code review: one call to the model configured in Bark, plus Bark's check if it is on. It costs money and takes a minute or so. Returns the findings as JSON; findings Bark's check doubts or that repeat a point an earlier review of the pull request raised or someone dismissed there are kept, set aside, with doubt, raised_before or dismissed saying why; unchecked says why the check didn't check one. Bark's fail_on setting doesn't apply. Nothing is posted anywhere."
	if len(opts.Reviewers) > 0 {
		desc += " Saved reviewers: " + strings.Join(opts.Reviewers, ", ") + "."
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "review",
		Title:       "Bark code review",
		Description: desc,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(true)},
	}, opts.review)
}

func (o Options) review(ctx context.Context, _ *mcp.CallToolRequest, in ReviewInput) (*mcp.CallToolResult, ReviewOutput, error) {
	// The source the same way as review_diff's.
	ropts, err := sourceOptions(DiffInput{Source: in.Source, Ref: in.Ref, WithPRDescription: in.WithPRDescription})
	if err != nil {
		return nil, ReviewOutput{}, err
	}

	var log bytes.Buffer
	ropts.ReviewerName = in.Reviewer
	ropts.Instructions = in.Instructions
	ropts.Paths = o.Paths
	ropts.Config = o.Config
	ropts.Structured = true
	ropts.Check = in.Check
	// The review comes back as events; its Markdown isn't needed.
	ropts.Out = io.Discard
	ropts.Log = &log

	res, err := o.Review(ctx, ropts)
	var failOn *plain.FailOnError
	var nothing *plain.NothingToReviewError
	switch {
	case errors.As(err, &failOn):
		// fail_on is for CI's exit status. The agent gets the findings,
		// and Bark's notes say which reach it.
		err = nil
	case errors.Is(err, plain.ErrNoReviewer):
		err = o.noReviewerError()
	case errors.As(err, &nothing):
		err = nothingToReviewError(nothing, in.Source)
	}

	ns := notes(log.String())
	// The user's credentials pay for a model the repository picked: the
	// agent is told, as the CLI's user is.
	if w := o.Config.RepoLLMWarning(); w != "" {
		ns = append([]string{"warning: " + w}, ns...)
	}
	if err != nil {
		if len(ns) > 0 {
			err = fmt.Errorf("%w (%s)", err, strings.Join(ns, "; "))
		}
		return nil, ReviewOutput{}, err
	}

	out := ReviewOutput{Findings: []ReviewFinding{}, Checked: res.Checked, Notes: ns}
	var text []string
	for _, e := range res.Events {
		switch {
		case e.Intro != "":
			out.Intro = e.Intro
		case e.Verdict != "":
			out.Verdict = e.Verdict
		case e.Finding != nil:
			out.Findings = append(out.Findings, reviewFinding(*e.Finding))
		case e.Text != "":
			text = append(text, e.Text)
		}
	}
	out.Text = strings.Join(text, "\n")
	return nil, out, nil
}

func (o Options) noReviewerError() error {
	if len(o.Reviewers) == 0 {
		return errors.New("reviewer is required: pass a reviewer file's path, or add .bark/reviewer.md to the repository (bark add saves reviewers)")
	}
	return fmt.Errorf("reviewer is required: pass one of %s, or add .bark/reviewer.md to the repository", strings.Join(o.Reviewers, ", "))
}

// nothingToReviewError is e with its advice in the tool's terms instead of
// the CLI's flags.
func nothingToReviewError(e *plain.NothingToReviewError, source string) error {
	if e.Advice == "" {
		return e
	}
	var advice string
	switch source {
	case sourceStaged:
		advice = `stage some with git add, or use source "changes" to review every uncommitted change`
	case sourceBranch:
		advice = `use source "changes" or "staged" to review them`
	default:
		advice = `use source "branch" with the base branch as ref to review this branch's commits, or source "commit" with a hash as ref for one commit`
	}
	return &plain.NothingToReviewError{Reason: e.Reason, Advice: advice}
}

func reviewFinding(f findings.Finding) ReviewFinding {
	return ReviewFinding{
		File:         f.File,
		Line:         f.Line,
		EndLine:      f.EndLine,
		Severity:     string(f.Severity),
		Category:     f.Category,
		Title:        f.Title,
		Body:         f.Body,
		Original:     f.Original,
		Suggestion:   f.Suggestion,
		Doubt:        f.Doubt,
		RaisedBefore: f.RaisedBefore,
		Dismissed:    f.Dismissed,
		Unchecked:    f.Unchecked,
		Repaired:     f.Repaired,
	}
}

// notes returns Bark's log lines without their "bark: " prefix.
func notes(log string) []string {
	var out []string
	for line := range strings.SplitSeq(log, "\n") {
		if line = strings.TrimSpace(strings.TrimPrefix(line, "bark: ")); line != "" {
			out = append(out, line)
		}
	}
	return out
}
