// Package mcpserver serves Bark's tools over the Model Context Protocol, so
// agents such as Claude Code, Cursor or Zed can call them. The server talks
// over stdin and stdout: nothing it runs may write to stdout.
package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/enclosing"
	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/ionut-t/bark/v2/internal/paths"
	"github.com/ionut-t/bark/v2/internal/plain"
	"github.com/ionut-t/bark/v2/internal/prompt"
	"github.com/ionut-t/bark/v2/internal/prreview"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// callTimeout caps the git and GitHub work of one tool call.
const callTimeout = 30 * time.Second

const instructions = `Bark is a code review tool.
review runs Bark's reviewer, a model call to the provider configured in Bark (it costs money and takes a minute or so), and returns its findings as JSON, after Bark's check where it is on. It is a second opinion from another model and the same review Bark's CI gives.
The other tools make no model calls and cost nothing.
review_diff returns a change the way Bark's reviewer sees it: files matched by .barkignore and .gitignore left out, with commits, the diffstat, the enclosing declarations of changed lines and related code from elsewhere in the repository.
earlier_findings returns what Bark's earlier reviews of a GitHub pull request raised, so the same points aren't raised again.`

// Options configures the server.
type Options struct {
	// Version is Bark's version, reported to clients.
	Version string
	// Limit caps the diff's size; a zero Limit means no cap.
	Limit git.DiffLimit

	// Config and Paths are what the review tool runs with; without a
	// Config there is no review tool. Reviewers are the saved reviewers'
	// names, listed in its description.
	Config    config.Config
	Paths     paths.Paths
	Reviewers []string

	// The functions below default to Bark's own; tests replace them.
	ReviewDiff func(context.Context, git.ReviewDiffParams) (git.ReviewDiff, error)
	Context    func(context.Context, git.ReviewDiff) (string, error)
	Earlier    func(context.Context, string) ([]findings.Finding, error)
	Review     func(context.Context, plain.ReviewOptions) (plain.ReviewResult, error)
}

// New returns a server with Bark's tools.
func New(opts Options) *mcp.Server {
	if opts.ReviewDiff == nil {
		opts.ReviewDiff = git.GetReviewDiff
	}
	if opts.Context == nil {
		opts.Context = enclosing.ForReview
	}
	if opts.Earlier == nil {
		opts.Earlier = prreview.Earlier
	}
	if opts.Review == nil {
		opts.Review = plain.Review
	}

	server := mcp.NewServer(
		&mcp.Implementation{Name: "bark", Title: "Bark", Version: opts.Version},
		&mcp.ServerOptions{Instructions: instructions},
	)

	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "review_diff",
		Title:       "Diff for review",
		Description: "Returns a change the way Bark's reviewer sees it: the diff without files matched by .barkignore or .gitignore, plus commits, the diffstat, and (unless without_context) the enclosing declarations of changed lines and related code. A diff longer than Bark's max_diff_lines setting ends with \"... (truncated)\". No model call.",
		Annotations: readOnly,
	}, opts.reviewDiff)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "earlier_findings",
		Title:       "Earlier findings on a pull request",
		Description: "Returns the findings that Bark's earlier reviews posted on a GitHub pull request recorded, oldest first, with what became of them: doubt (Bark's check doubted it), dismissed (someone dismissed it; don't raise it again), resolved (its thread was resolved, and by whom) and fixed (Bark's check found it fixed). Needs the gh CLI. No model call.",
		Annotations: readOnly,
	}, opts.earlierFindings)

	if opts.Config != nil {
		addReviewTool(server, opts)
	}

	return server
}

// Run serves Bark's tools over stdin and stdout until the client disconnects
// or ctx ends.
func Run(ctx context.Context, opts Options) error {
	return New(opts).Run(ctx, &mcp.StdioTransport{})
}

// Diff sources for review_diff.
const (
	sourceChanges = "changes"
	sourceStaged  = "staged"
	sourceBranch  = "branch"
	sourceCommit  = "commit"
	sourcePR      = "pr"
)

// DiffInput is review_diff's arguments.
type DiffInput struct {
	Source            string `json:"source,omitempty" jsonschema:"what to diff: changes (all uncommitted changes, the default), staged, branch (the current branch against ref), commit (the commit ref) or pr (GitHub pull request ref)"`
	Ref               string `json:"ref,omitempty" jsonschema:"the base branch for source branch, a commit hash for commit, a pull request number, URL or branch for pr"`
	WithPRDescription bool   `json:"with_pr_description,omitempty" jsonschema:"include the pull request's description (source pr only)"`
	WithoutContext    bool   `json:"without_context,omitempty" jsonschema:"leave out enclosing declarations and related code"`
}

func (o Options) reviewDiff(ctx context.Context, _ *mcp.CallToolRequest, in DiffInput) (*mcp.CallToolResult, any, error) {
	source, err := sourceOptions(in)
	if err != nil {
		return nil, nil, err
	}
	params := plain.DiffParams(source).WithLimit(o.Limit)

	diffCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	r, err := o.ReviewDiff(diffCtx, params)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(r.Diff) == "" {
		return nil, nil, fmt.Errorf("no changes to review (files matched by .barkignore or .gitignore are left out)")
	}

	var related string
	if !in.WithoutContext {
		// Its own timeout, as in the CLI, so a slow diff doesn't leave it
		// none. Without context the diff is still useful, but the agent is
		// told the section is missing, not that there is none.
		relatedCtx, cancel := context.WithTimeout(ctx, callTimeout)
		defer cancel()
		var err error
		if related, err = o.Context(relatedCtx, r); err != nil {
			related = fmt.Sprintf("_Related code unavailable: %v_\n\n", err)
		}
	}

	text := prompt.FormatReviewContent(r.ContextHeader, r.Stat, r.Commits, r.Diff, related)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}

// sourceOptions checks in's source and ref and returns them as the
// review options that select the same diff; plain.DiffParams turns those
// into the diff, for review_diff and review alike.
func sourceOptions(in DiffInput) (plain.ReviewOptions, error) {
	if in.WithPRDescription && in.Source != sourcePR {
		return plain.ReviewOptions{}, fmt.Errorf("with_pr_description needs source %q", sourcePR)
	}

	switch in.Source {
	case "", sourceChanges, sourceStaged:
		if in.Ref != "" {
			source := fmt.Sprintf("%q", in.Source)
			if in.Source == "" {
				source = fmt.Sprintf("%q (the default)", sourceChanges)
			}
			return plain.ReviewOptions{}, fmt.Errorf("source %s takes no ref; set source to branch, commit or pr", source)
		}
		return plain.ReviewOptions{Staged: in.Source == sourceStaged}, nil
	case sourceBranch, sourceCommit, sourcePR:
	default:
		return plain.ReviewOptions{}, fmt.Errorf("unknown source %q: use changes, staged, branch, commit or pr", in.Source)
	}

	if in.Ref == "" {
		return plain.ReviewOptions{}, fmt.Errorf("source %q needs ref", in.Source)
	}
	if err := notAnOption("ref", in.Ref); err != nil {
		return plain.ReviewOptions{}, err
	}
	switch in.Source {
	case sourceBranch:
		return plain.ReviewOptions{Branch: in.Ref}, nil
	case sourceCommit:
		return plain.ReviewOptions{Hash: in.Ref}, nil
	default:
		return plain.ReviewOptions{PR: in.Ref, WithPRDescription: in.WithPRDescription}, nil
	}
}

// notAnOption returns an error if value, an argument named name, starts
// with "-". git and gh get it as an argument, and would take it as an
// option: git show --output=<file> writes a file. A branch name can't start
// with "-", nor can a hash, a pull request number or a URL.
func notAnOption(name, value string) error {
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("%s %q can't start with \"-\"", name, value)
	}
	return nil
}

// EarlierInput is earlier_findings' arguments.
type EarlierInput struct {
	PR string `json:"pr" jsonschema:"the pull request: a number, URL or branch"`
}

// EarlierOutput is earlier_findings' result.
type EarlierOutput struct {
	Findings []EarlierFinding `json:"findings"`
}

// EarlierFinding is what Bark records of a finding it posted: no severity or
// category, which aren't recorded.
type EarlierFinding struct {
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	EndLine int    `json:"end_line,omitempty"`
	Title   string `json:"title"`
	Body    string `json:"body,omitempty"`
	// Doubt is set when Bark's check doubted the point in the review that
	// raised it.
	Doubt string `json:"doubt,omitempty"`
	// Dismissed says how someone dismissed the point on the pull request.
	Dismissed string `json:"dismissed,omitempty"`
	// Resolved says who resolved the point's thread: someone, or Bark once
	// its check found the point fixed.
	Resolved string `json:"resolved,omitempty"`
	// Fixed means Bark's check found the point fixed; raising it again is
	// a regression.
	Fixed bool `json:"fixed,omitempty"`
}

func (o Options) earlierFindings(ctx context.Context, _ *mcp.CallToolRequest, in EarlierInput) (*mcp.CallToolResult, EarlierOutput, error) {
	if in.PR == "" {
		return nil, EarlierOutput{}, fmt.Errorf("pr is required")
	}
	if err := notAnOption("pr", in.PR); err != nil {
		return nil, EarlierOutput{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	prior, err := o.Earlier(ctx, in.PR)
	if err != nil {
		return nil, EarlierOutput{}, err
	}
	// An empty list, not null, so the output matches its schema.
	out := EarlierOutput{Findings: make([]EarlierFinding, 0, len(prior))}
	for _, f := range prior {
		out.Findings = append(out.Findings, EarlierFinding{
			File: f.File, Line: f.Line, EndLine: f.EndLine, Title: f.Title, Body: f.Body,
			Doubt: f.Doubt, Dismissed: f.Dismissed, Resolved: f.Resolved, Fixed: f.Fixed,
		})
	}
	return nil, out, nil
}
