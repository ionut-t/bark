package plain

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ionut-t/bark/v2/internal/check"
	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/debuglog"
	"github.com/ionut-t/bark/v2/internal/enclosing"
	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/ionut-t/bark/v2/internal/git"
	"github.com/ionut-t/bark/v2/internal/houserules"
	"github.com/ionut-t/bark/v2/internal/instructions"
	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/ionut-t/bark/v2/internal/llm/llm_factory"
	"github.com/ionut-t/bark/v2/internal/paths"
	"github.com/ionut-t/bark/v2/internal/prices"
	"github.com/ionut-t/bark/v2/internal/prompt"
	"github.com/ionut-t/bark/v2/internal/prreview"
	"github.com/ionut-t/bark/v2/internal/reviewers"
	"github.com/ionut-t/bark/v2/internal/utils"
)

const gitTimeout = 30 * time.Second

// ReviewOptions configures the plain text review runner.
type ReviewOptions struct {
	Diff              *string
	ReviewerName      string
	Instructions      []string
	SkipInstruction   bool
	Paths             paths.Paths
	Config            config.Config
	Stream            bool
	WithPRDescription bool
	// Comment posts the review to PR, with inline comments on the lines its
	// findings are about. It needs PR and a structured review. A pull
	// request Bark reviewed before is reviewed only from the commit the
	// last review was made at, unless Full is set.
	Comment bool
	// Full reviews the whole pull request even when Bark reviewed part of
	// it before.
	Full bool
	// Approve posts the review as an approval when it keeps no finding at
	// or above ApproveBelow; see approval. It needs Comment.
	Approve bool
	// ApproveBelow is the severity from which a finding stops the review
	// from approving: medium when empty, or fail_on when that's lower.
	ApproveBelow findings.Severity
	// NoResolve leaves open the threads of Bark's earlier inline comments
	// that its check finds fixed. Without it, a posted review resolves
	// them, after replying on each with the check's reason.
	NoResolve bool

	// Diff source flags (used when Diff is empty)
	Staged bool
	All    bool
	Branch string
	Hash   string
	PR     string
}

// CommitOptions configures the plain text commit runner.
type CommitOptions struct {
	Diff   *string
	All    bool
	Hint   string
	Config config.Config
}

// PROptions configures the plain text PR runner.
type PROptions struct {
	Diff         *string
	Branch       string
	PR           string
	Instructions []string
	Config       config.Config
}

// RunReview runs a code review and writes the output to stdout.
func RunReview(opts ReviewOptions) error {
	if opts.Comment && (opts.PR == "" || opts.Diff != nil) {
		return fmt.Errorf("posting the review needs a pull request diff (--pr), not a diff from stdin")
	}
	if opts.Approve && !opts.Comment {
		return fmt.Errorf("approving needs the review to be posted (--comment)")
	}
	if opts.ApproveBelow != "" && !opts.Approve {
		return fmt.Errorf("approve_below needs approving to be on (--approve)")
	}
	if opts.NoResolve && !opts.Comment {
		return fmt.Errorf("leaving fixed threads open (--no-resolve) needs the review to be posted (--comment)")
	}

	// fail_on needs findings to count, so it turns on structured findings
	// as --comment does.
	failOn := findings.Severity(opts.Config.GetFailOn())
	structured := opts.Config.GetStructuredFindings() || opts.Comment || failOn != ""
	checking := structured && opts.Config.GetCheckFindings()

	// What Bark's earlier reviews of the pull request recorded: the
	// findings the check sets repeats of aside, and the commit to review
	// from when posting.
	incremental := opts.Comment && opts.Diff == nil && !opts.Full
	var hist prreview.History
	histOK := false
	if opts.PR != "" && (incremental || checking) {
		hist, histOK = readHistory(opts.PR, checking, incremental)
	}

	var reviewDiff git.ReviewDiff

	if opts.Diff == nil {
		gitCtx, gitCancel := context.WithTimeout(context.Background(), gitTimeout)
		defer gitCancel()

		limit := git.LimitFrom(opts.Config)
		var diffParams git.ReviewDiffParams
		switch {
		case opts.PR != "":
			diffParams = git.PRDiff(opts.PR).WithLimit(limit)
			if opts.WithPRDescription {
				diffParams = diffParams.WithPRDescription()
			}
			if incremental && hist.Head != "" {
				diffParams = diffParams.Since(hist.Head, hist.Base)
			}
		case opts.Branch != "":
			diffParams = git.BranchDiff(opts.Branch).WithLimit(limit)
		case opts.Hash != "":
			diffParams = git.CommitDiff(opts.Hash).WithLimit(limit)
		default:
			diffParams = git.WorkingTreeDiff(opts.Staged).WithLimit(limit)
		}

		var err error
		reviewDiff, err = git.GetReviewDiff(gitCtx, diffParams)
		if err != nil {
			return err
		}
		warnReviewDiff(os.Stderr, reviewDiff)
		if opts.Comment && (histOK || opts.Full) {
			noteReviewScope(os.Stderr, opts.Full, hist.Head, reviewDiff)
		}
		if reviewDiff.Since != "" && reviewDiff.Diff == "" {
			// On stdout too, so CI can tell it from a review that failed
			// to produce anything.
			fmt.Println(nothingToReview(reviewDiff))
			return nil
		}
	} else {
		reviewDiff.Diff = *opts.Diff
	}

	if reviewDiff.Diff == "" {
		return fmt.Errorf("no diff content available")
	}

	reviewer, err := resolveReviewer(opts.ReviewerName, opts.Paths.Config)
	if err != nil {
		return err
	}

	var reviewInstructions string
	if !opts.SkipInstruction {
		var err error
		reviewInstructions, err = resolveInstructions(opts.Instructions, opts.Paths.Config)
		if err != nil {
			return err
		}
	}
	minSeverity := findings.Severity(opts.Config.GetMinSeverity())
	warnFailOnBelowMin(os.Stderr, failOn, minSeverity)
	var approveThreshold findings.Severity
	if opts.Approve {
		approveThreshold = approvalThreshold(os.Stderr, opts.ApproveBelow, failOn)
	}
	var rules houserules.Rules
	if opts.Config.GetHouseRules() {
		rulesCtx, rulesCancel := context.WithTimeout(context.Background(), gitTimeout)
		read := reviewDiff.RepoFile
		if opts.Diff != nil {
			// A diff from stdin has no version; read the working tree.
			read = func(ctx context.Context, path string) ([]byte, error) {
				return git.GetFileContent(ctx, "", path)
			}
		}
		rules = houserules.ForReview(rulesCtx, read, reviewDiff.Diff, opts.Config.GetHouseRulesFiles())
		rulesCancel()
		for _, w := range rules.Warnings() {
			fmt.Fprintf(os.Stderr, "bark: warning: %s\n", w)
		}
	}
	system := prompt.FormatReviewSystem(reviewer.Prompt, reviewInstructions, rules.Text)
	if structured {
		system = prompt.FormatStructuredReviewSystem(reviewer.Prompt, reviewInstructions, rules.Text)
	}

	var enclosingContext string
	if opts.Config.GetContextEnrichment() {
		enclosingCtx, enclosingCancel := context.WithTimeout(context.Background(), gitTimeout)
		var err error
		enclosingContext, err = enclosing.ForReview(enclosingCtx, reviewDiff)
		enclosingCancel()
		if err != nil {
			// Gracefully continue without enclosing context if extraction fails
			enclosingContext = ""
		}
	}

	diff := reviewDiff.Diff
	if structured {
		diff = prompt.NumberDiffLines(diff)
	}
	promptText := prompt.FormatReviewContent(reviewDiff.ContextHeader, reviewDiff.Stat, reviewDiff.Commits, diff, enclosingContext)

	client, provider, err := llm_factory.New(context.Background(), opts.Config)
	if err != nil {
		return fmt.Errorf("error creating LLM client: %w", err)
	}

	llmCtx, llmCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer llmCancel()

	model, _ := opts.Config.GetLLMModel()
	client, meter := meterCalls(client, opts.Paths.Cache, provider, model)
	defer printCost(meter)

	debugLog, err := debuglog.Start(opts.Paths.State, "review", debuglog.Meta{
		Fields: []debuglog.Field{
			{Key: "provider", Value: provider},
			{Key: "model", Value: model},
			{Key: "reviewer", Value: reviewer.Name},
			{Key: "structured", Value: strconv.FormatBool(structured)},
			{Key: "house_rules", Value: houserules.LogField(opts.Config.GetHouseRules(), rules)},
		},
		System:  system,
		Prompt:  promptText,
		Pricing: meter.Pricing(),
	})
	// Stderr, so the review on stdout stays clean for pipes.
	if err != nil {
		fmt.Fprintf(os.Stderr, "bark: debug log: %v\n", err)
	} else if debugLog != nil {
		fmt.Fprintf(os.Stderr, "bark: debug log: %s\n", debugLog.Path())
	}

	out := io.Writer(os.Stdout)
	// Before the review, so a streamed one starts with it too.
	if note := partialReviewNote(reviewDiff.Cut); note != "" {
		fmt.Fprintf(os.Stderr, "bark: warning: %s\n", reviewDiff.Cut.Summary())
		_, _ = fmt.Fprintln(out, note)
	}
	var output string
	var truncated bool
	if opts.Stream {
		output, truncated, err = streamResponse(llmCtx, client, system, promptText, out, structured, minSeverity, debugLog)
	} else {
		output, truncated, err = fullResponse(llmCtx, client, system, promptText, debugLog)
	}
	meter.Mark("review")
	if err != nil {
		return err
	}
	if truncated {
		warnTruncated("review", "it ends early and may be missing findings")
	}

	// The threads of Bark's earlier inline comments that its check may
	// find fixed, and resolve once the review is posted.
	maps := newLineMaps(reviewDiff.PRHead())
	var threads []prreview.Thread
	var toCheck []check.Thread
	if opts.Comment && !opts.NoResolve && histOK {
		threads, toCheck = threadsToCheck(os.Stderr, hist, checking, reviewDiff.PRHead(), maps)
	}
	var fixed map[int]string

	var checkFn func([]findings.Event) ([]findings.Event, int)
	if checking {
		r := check.Review{Client: client, ReviewDiff: reviewDiff, Diff: diff, HouseRules: rules.Text, LogDir: opts.Paths.State, Provider: provider, Model: model, Pricing: meter.Pricing()}
		if !opts.Stream {
			// Nothing is shown before the check, so the intro and verdict
			// can still be rewritten to match it.
			r.Persona = reviewer.Prompt
		}
		r.Prior = hist.Findings
		r.SameLines = sameLinesHint(os.Stderr, hist.Findings, lineMapper(maps))
		r.Threads = toCheck
		checkFn = func(events []findings.Event) ([]findings.Event, int) {
			var checked int
			events, checked, fixed = checkFindings(r, events, meter)
			return events, checked
		}
	}
	events := finishReview(out, output, truncated, opts.Stream, structured, minSeverity, checkFn)

	var post func() error
	if opts.Comment {
		post = func() error {
			scope := postScope(reviewDiff, truncated)
			// Before approving: a point fixed and resolved no longer
			// holds an incremental approval back.
			hist.MarkResolved(resolveFixed(os.Stderr, fixedThreads(threads, fixed), scope.Head), scope.Head)
			approve := false
			if opts.Approve {
				var why string
				approve, why = approval(events, scope, hist, approveThreshold)
				if !approve {
					fmt.Fprintf(os.Stderr, "bark: not approving: %s\n", why)
				}
			}
			// An approval changes the pull request's review state, so it
			// is posted even when the review adds no new finding.
			if !approve && skipPost(os.Stderr, events, scope, hist) {
				return nil
			}
			postCtx, postCancel := context.WithTimeout(context.Background(), gitTimeout)
			defer postCancel()
			result, err := prreview.Post(postCtx, opts.PR, withPartialNote(events, reviewDiff.Cut), reviewDiff.CommentDiff(),
				scope, approve)
			if err != nil {
				return err
			}
			if result.Fallback {
				fmt.Fprintln(os.Stderr, "bark: GitHub rejected the inline comments; posted every finding in the review body instead")
			}
			if result.ApprovalErr != nil {
				fmt.Fprintf(os.Stderr, "bark: warning: GitHub refused to approve the pull request, so the review was posted as a comment: %v\n", result.ApprovalErr)
			}
			kind := "review"
			if result.Approved {
				kind = "approving review"
			}
			fmt.Fprintf(os.Stderr, "bark: posted %s with %d inline comment(s): %s\n", kind, result.Inline, result.URL)
			return nil
		}
	}
	return concludeReview(os.Stderr, events, failOn, post)
}

// FailOnExitCode is the exit status of a review that fails its fail_on
// threshold. Every other error exits 1, so CI can tell a blocked merge from
// Bark failing.
const FailOnExitCode = 3

// FailOnError is returned by a review that kept findings at or above its
// fail_on severity.
type FailOnError struct {
	Threshold findings.Severity
	Findings  []findings.Finding
}

func (e *FailOnError) Error() string {
	return fmt.Sprintf("%d finding(s) at or above %s severity (fail_on %s)", len(e.Findings), e.Threshold, e.Threshold)
}

// concludeReview posts the review (if post is not nil), then fails it if
// it kept findings at or above failOn. Posting comes first, so a review
// that blocks a merge is still on the pull request; a failed post is
// returned instead of the gate's result.
func concludeReview(w io.Writer, events []findings.Event, failOn findings.Severity, post func() error) error {
	if post != nil {
		if err := post(); err != nil {
			return err
		}
	}
	return gate(w, events, failOn)
}

// gate returns a *FailOnError if events hold findings at or above failOn
// that Bark's check didn't doubt or find dismissed, and lists them on w. Repeats of points
// raised before count, since they're still in the code; when the check
// didn't run or failed, every finding counts.
func gate(w io.Writer, events []findings.Event, failOn findings.Severity) error {
	blocking := findings.Blocking(events, failOn)
	if len(blocking) == 0 {
		return nil
	}
	err := &FailOnError{Threshold: failOn, Findings: blocking}
	_, _ = fmt.Fprintf(w, "bark: failing: %s:\n", err.Error())
	for _, f := range blocking {
		where := f.File
		if where != "" && f.Line > 0 {
			where += ":" + strconv.Itoa(f.Line)
		}
		if where != "" {
			where += " "
		}
		note := ""
		if f.RaisedBefore != "" {
			note = " (raised before, still there)"
		}
		_, _ = fmt.Fprintf(w, "bark:   [%s] %s%s%s\n", strings.ToLower(strings.TrimSpace(string(f.Severity))), where, f.Title, note)
	}
	return err
}

// approvalThreshold is the severity from which a finding stops a review
// from approving: approveBelow, medium when empty, so findings below it
// (nits) are left as comments on an approval. A review that fails its
// fail_on gate doesn't approve, so a lower fail_on wins; when approveBelow
// was set above it, it says so on w.
func approvalThreshold(w io.Writer, approveBelow, failOn findings.Severity) findings.Severity {
	threshold := approveBelow
	if threshold == "" {
		threshold = findings.Medium
	}
	if failOn == "" || failOn.AtLeast(threshold) {
		return threshold
	}
	if approveBelow != "" {
		_, _ = fmt.Fprintf(w, "bark: warning: approve_below %s is above fail_on %s; a review with a finding at %s or above fails, so it doesn't approve either\n",
			approveBelow, failOn, failOn)
	}
	return failOn
}

// approval reports whether a review of scope with events, posted on a pull
// request whose earlier reviews recorded hist, approves it, or why not. It
// approves only a complete review (the whole diff seen, a verdict given,
// every finding read) that keeps no finding at or above threshold, counted
// as fail_on counts them. A review of only the changes since the last one
// doesn't see the code earlier findings were about, so it also needs every
// point raised earlier to have been dismissed, doubted by Bark's check, or
// resolved (by someone, or by Bark once its check found it fixed).
func approval(events []findings.Event, scope prreview.Scope, hist prreview.History, threshold findings.Severity) (bool, string) {
	if scope.Partial {
		return false, "the review didn't see the whole change (the diff was cut to fit its limit, or the model's output ended early)"
	}
	if !slices.ContainsFunc(events, func(e findings.Event) bool { return e.Verdict != "" }) {
		return false, "the review has no verdict, so it may be incomplete"
	}
	if slices.ContainsFunc(events, func(e findings.Event) bool { return strings.HasPrefix(strings.TrimSpace(e.Text), "{") }) {
		return false, "Bark couldn't read one of the review's findings"
	}
	if blocking := findings.Blocking(events, threshold); len(blocking) > 0 {
		return false, fmt.Sprintf("the review kept %d finding(s) at or above %s severity", len(blocking), threshold)
	}
	if scope.Since != "" {
		open := 0
		for _, f := range hist.Findings {
			if f.Dismissed == "" && f.Doubt == "" && f.Resolved == "" {
				open++
			}
		}
		if open > 0 {
			return false, fmt.Sprintf("it reviewed only the changes since the last review, and %d point(s) raised earlier on the pull request may still be open (review the whole pull request to check them again, or dismiss or resolve them)", open)
		}
	}
	return true, ""
}

// warnFailOnBelowMin says on w when fail_on is less severe than
// min_severity: findings below min_severity are dropped before the gate
// sees them, so the effective threshold is min_severity.
func warnFailOnBelowMin(w io.Writer, failOn, minSeverity findings.Severity) {
	if failOn == "" || minSeverity == "" || failOn.AtLeast(minSeverity) {
		return
	}
	_, _ = fmt.Fprintf(w, "bark: warning: fail_on %s is below min_severity %s; findings below %s are left out, so only those at %s or above can fail the review\n",
		failOn, minSeverity, minSeverity, minSeverity)
}

// RunCommit generates a commit message and writes it to stdout.
func RunCommit(opts CommitOptions) error {
	var diff, branch string

	if opts.Diff == nil {
		gitCtx, gitCancel := context.WithTimeout(context.Background(), gitTimeout)
		defer gitCancel()

		var err error
		diff, err = git.GetWorkingTreeDiff(gitCtx, opts.All)
		if err != nil {
			return err
		}
		// A piped diff may not come from the current branch, so only name it
		// when the diff is the working tree's.
		branch, _ = git.GetCurrentBranch(gitCtx)
	} else {
		diff = *opts.Diff
	}

	if diff == "" {
		return fmt.Errorf("no changes to generate a commit message for")
	}

	commitInstructions, err := utils.GetInstructions(".bark/commit.md", opts.Config.GetCommitInstructions())
	if err != nil {
		return err
	}
	commitSystem := prompt.FormatCommitSystem(commitInstructions, opts.Hint)

	client, meter, err := newMeteredClient(opts.Config)
	if err != nil {
		return err
	}
	defer printCost(meter)

	llmCtx, llmCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer llmCancel()

	result, err := client.Generate(llmCtx, commitSystem, prompt.FormatCommitContent(branch, diff))
	if err != nil {
		return fmt.Errorf("error generating commit message: %w", err)
	}
	if result.Truncated {
		warnTruncated("commit message", "it may be incomplete")
	}

	fmt.Print(utils.RemoveCodeFences(result.Content))
	fmt.Println()

	return nil
}

// RunPR generates a PR description and writes it to stdout.
func RunPR(opts PROptions) error {
	prInstructions, err := resolvePRInstructions(opts.Instructions, opts.Config)
	if err != nil {
		return err
	}

	var content string

	switch {
	case opts.Diff != nil:
		content = *opts.Diff
	default:
		gitCtx, gitCancel := context.WithTimeout(context.Background(), gitTimeout)
		defer gitCancel()

		var err error
		if opts.PR != "" {
			content, err = git.GetPRInfo(gitCtx, opts.PR)
		} else {
			var branchInfo *git.BranchInfo
			branchInfo, err = git.GetBranchInfo(gitCtx, opts.Branch, git.LimitFrom(opts.Config))
			if err == nil {
				content = git.FormatBranchInfo(branchInfo)
			}
		}
		if err != nil {
			return err
		}
	}

	prSystem := prompt.FormatPRSystem(prInstructions)

	client, meter, err := newMeteredClient(opts.Config)
	if err != nil {
		return err
	}
	defer printCost(meter)

	llmCtx, llmCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer llmCancel()

	result, err := client.Generate(llmCtx, prSystem, content)
	if err != nil {
		return fmt.Errorf("error generating PR description: %w", err)
	}
	if result.Truncated {
		warnTruncated("PR description", "it may be incomplete")
	}

	fmt.Print(result.Content)
	fmt.Println()

	return nil
}

// streamResponse streams the review to out and returns the raw model output,
// and whether the model cut it off at its output limit.
// A structured review is rendered to Markdown as it arrives, without the
// findings less severe than minSeverity.
func streamResponse(ctx context.Context, client llm.LLM, system, promptText string, out io.Writer, structured bool, minSeverity findings.Severity, debugLog *debuglog.Log) (output string, truncated bool, err error) {
	responseChan, errChan := client.Stream(ctx, system, promptText)

	renderer := findings.Renderer{Min: minSeverity}
	var text strings.Builder
	for chunk := range responseChan {
		text.WriteString(chunk.Content)
		truncated = truncated || chunk.Truncated
		debugLog.Write(chunk.Content, chunk.Usage)
		if structured {
			_, _ = fmt.Fprint(out, renderer.Write(chunk.Content))
		} else {
			_, _ = fmt.Fprint(out, chunk.Content)
		}
	}
	if structured {
		_, _ = fmt.Fprint(out, renderer.Close())
	}
	_, _ = fmt.Fprintln(out)

	err = <-errChan
	debugLog.Finish(err)
	if err != nil {
		return "", false, fmt.Errorf("error during review: %w", err)
	}

	return text.String(), truncated, nil
}

// finishReview writes what's left of the review to out once the model is
// done, and returns its events (nil for a free-text review). A streamed
// review is already on out; a whole one is written now. checkFn, if not nil,
// runs Bark's check on a structured review's findings: a whole review shows
// the doubted findings in a section of their own, a streamed one gets a
// report of them appended. Findings less severe than minSeverity are left
// out before the check, so they cost nothing to check. A truncated review
// ends with findings.TruncatedNote; a structured one also gets it as its last
// event, so a posted review says so too.
func finishReview(out io.Writer, output string, truncated, stream, structured bool, minSeverity findings.Severity, checkFn func([]findings.Event) ([]findings.Event, int)) []findings.Event {
	if !structured {
		if !stream {
			_, _ = fmt.Fprintln(out, output)
		}
		if truncated {
			_, _ = fmt.Fprint(out, "\n"+findings.TruncatedNote)
		}
		return nil
	}

	events, below := findings.DropBelow(findings.Parse(output), minSeverity)
	if below > 0 {
		fmt.Fprintf(os.Stderr, "bark: left out %d finding(s) below %s severity (min_severity)\n", below, minSeverity)
	}
	checked := 0
	if checkFn != nil {
		events, checked = checkFn(events)
	}
	if truncated {
		events = append(events, findings.Event{Text: findings.TruncatedNote})
	}
	if stream {
		_, _ = fmt.Fprint(out, findings.CheckReport(events, checked))
		if truncated {
			_, _ = fmt.Fprint(out, findings.TruncatedNote)
		}
	} else {
		_, _ = fmt.Fprintln(out, findings.Markdown(events))
	}
	return events
}

// fullResponse returns the whole review once it is complete, and whether the
// model cut it off at its output limit.
func fullResponse(ctx context.Context, client llm.LLM, system, promptText string, debugLog *debuglog.Log) (output string, truncated bool, err error) {
	response, err := client.Generate(ctx, system, promptText)
	debugLog.Write(response.Content, response.Usage)
	debugLog.Finish(err)
	if err != nil {
		return "", false, fmt.Errorf("error during review: %w", err)
	}
	return response.Content, response.Truncated, nil
}

// checkFindings runs Bark's check on the findings in events, and on the open
// threads of r, and returns the events with the doubted findings marked,
// how many findings were checked, and the threads it found fixed (see
// check.Result.Fixed). Progress and errors go to stderr; if the check
// fails, events are returned with every finding marked unchecked.
func checkFindings(r check.Review, events []findings.Event, meter *prices.Meter) ([]findings.Event, int, map[int]string) {
	n := check.Count(events)
	if n == 0 && len(r.Threads) == 0 {
		return events, 0, nil
	}
	what := fmt.Sprintf("%d finding(s)", n)
	if len(r.Threads) > 0 {
		what += fmt.Sprintf(" and %d open thread(s) whose code changed", len(r.Threads))
	}
	fmt.Fprintf(os.Stderr, "bark: checking %s against the code...\n", what)

	r.OnLog = func(path string, err error) {
		if err != nil {
			fmt.Fprintf(os.Stderr, "bark: debug log: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "bark: debug log: %s\n", path)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	res, err := r.Run(ctx, events)
	meter.Mark("check")
	if err != nil {
		var left []string
		if n > 0 {
			left = append(left, "the findings are unchecked")
		}
		if len(r.Threads) > 0 {
			left = append(left, "no thread is resolved")
		}
		fmt.Fprintf(os.Stderr, "bark: %v; %s\n", err, strings.Join(left, ", and "))
		return findings.MarkUnchecked(events, "it failed."), 0, nil
	}
	if n > 0 {
		fmt.Fprint(os.Stderr, checkSummary(res))
	}
	if len(r.Threads) > 0 {
		fmt.Fprintf(os.Stderr, "bark: the check found %d of %d open thread(s) fixed\n", len(res.Fixed), len(r.Threads))
	}

	// The intro and verdict were written before the check; rewrite them so
	// they don't demand fixes for doubted findings.
	revised, ok, err := r.Revise(ctx, res.Events)
	meter.Mark("verdict rewrite")
	switch {
	case err != nil:
		fmt.Fprintf(os.Stderr, "bark: %v; keeping the original\n", err)
	case ok:
		fmt.Fprintln(os.Stderr, "bark: rewrote the intro and verdict to match the checked findings")
	}
	return revised, res.Checked, res.Fixed
}

// checkSummary is the stderr line saying what the check kept. Kept means
// counted for fail_on: neither doubted nor dismissed on the pull request.
// Repeats of points raised before are kept: they're still in the code,
// only not posted again. Findings the check gave no verdict on are kept
// unchecked.
func checkSummary(res check.Result) string {
	s := fmt.Sprintf("bark: the check kept %d of %d finding(s)", res.Checked-res.Doubted-res.Dismissed, res.Checked)
	if res.RaisedBefore > 0 {
		s += fmt.Sprintf("; %d raised before on this pull request, not posted again", res.RaisedBefore)
	}
	if res.Dismissed > 0 {
		s += fmt.Sprintf("; %d dismissed on this pull request, not posted", res.Dismissed)
	}
	if res.Unchecked > 0 {
		s += fmt.Sprintf("; it gave no verdict on %d finding(s), they're unchecked", res.Unchecked)
	}
	return s + "\n"
}

// sameLinesHint returns the check's hook that finds the findings on lines
// of a point dismissed on the pull request (in prior) that haven't changed
// since (see prreview.DismissedOnLines), warning on w about lookups that
// failed. It's nil without mapLines (no pull request head to compare with).
func sameLinesHint(w io.Writer, prior []findings.Finding, mapLines prreview.MapLines) func([]findings.Finding) map[int]findings.Finding {
	if mapLines == nil {
		return nil
	}
	return func(fs []findings.Finding) map[int]findings.Finding {
		hint, err := prreview.DismissedOnLines(fs, prior, mapLines)
		if err != nil {
			_, _ = fmt.Fprintf(w, "bark: warning: couldn't match some dismissed points by line: %v\n", err)
		}
		return hint
	}
}

// lineMapper maps lines from the commits of earlier reviews to head with
// maps. It's nil without a head.
func lineMapper(maps *lineMaps) prreview.MapLines {
	if maps.head == "" {
		return nil
	}
	// The lookups whose failure was reported.
	reported := map[string]bool{}
	return func(commit, path string, start, end int) (int, int, bool, error) {
		m, err := maps.get(commit, path)
		if err != nil {
			if key := commit + "\x00" + path; !reported[key] {
				reported[key] = true
				return 0, 0, false, err
			}
			return 0, 0, false, nil
		}
		s, e, ok := m.Map(start, end)
		return s, e, ok, nil
	}
}

// readHistory returns what Bark's earlier reviews of pull request pr
// recorded: the findings they raised, for the check to drop repeats of
// (when checking), and where the last one says the next review can start
// (when incremental). If they can't be read, the review goes on without
// them, and ok is false.
func readHistory(pr string, checking, incremental bool) (h prreview.History, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	h, err := prreview.ReadHistory(ctx, pr)
	historyNote(os.Stderr, h, err, checking, incremental)
	return h, err == nil
}

// historyNote says on w what reading the earlier reviews of a pull request
// means for this review, mentioning only what it uses them for.
func historyNote(w io.Writer, h prreview.History, err error, checking, incremental bool) {
	if err != nil {
		var without []string
		if checking {
			without = append(without, "repeats of earlier findings won't be set aside")
		}
		if incremental {
			without = append(without, "the whole pull request is reviewed")
		}
		if len(without) == 0 {
			_, _ = fmt.Fprintf(w, "bark: %v\n", err)
			return
		}
		_, _ = fmt.Fprintf(w, "bark: %v; %s\n", err, strings.Join(without, ", and "))
		return
	}
	if !checking {
		return
	}
	if len(h.Findings) > 0 {
		dismissed := ""
		if n := h.Dismissed(); n > 0 {
			dismissed = fmt.Sprintf("; %d dismissed", n)
		}
		_, _ = fmt.Fprintf(w, "bark: earlier reviews of this pull request raised %d finding(s)%s\n", len(h.Findings), dismissed)
	}
	if h.DismissalsErr != nil {
		_, _ = fmt.Fprintf(w, "bark: warning: %v; some dismissals (👎 or \"won't fix\") may be missed\n", h.DismissalsErr)
	}
}

// skipPost reports whether a review of scope with events should not be
// posted, because it adds nothing to the last review in hist (see
// prreview.NothingNew), and says so on w. The review is still shown, and
// fail_on still applies to it.
func skipPost(w io.Writer, events []findings.Event, scope prreview.Scope, hist prreview.History) bool {
	if !prreview.NothingNew(events, scope, hist) {
		return false
	}
	why := "no findings"
	for _, e := range events {
		if e.Finding != nil {
			why = "every finding repeats a point raised before or dismissed"
			break
		}
	}
	_, _ = fmt.Fprintf(w, "bark: not posted: nothing new since the last review at %s: %s\n", hist.Head[:min(7, len(hist.Head))], why)
	return true
}

// postScope is what a posted review of r covered. truncated says the
// model's output ended early.
func postScope(r git.ReviewDiff, truncated bool) prreview.Scope {
	return prreview.Scope{
		Head:    r.PRHead(),
		Base:    r.PRMergeBase(),
		Since:   r.Since,
		Partial: r.Cut != nil || truncated,
	}
}

// nothingToReview is the output of a review with nothing to review: no
// changes since the last review of r's pull request, or only to files
// Bark ignores.
func nothingToReview(r git.ReviewDiff) string {
	short := r.Since[:min(7, len(r.Since))]
	if len(r.Commits) > 0 {
		return fmt.Sprintf("Nothing to review: the changes since the last review at `%s` touch only files Bark ignores.", short)
	}
	return fmt.Sprintf("Nothing to review: nothing changed since the last review at `%s`.", short)
}

// noteReviewScope says on w whether the review of a pull request covers it
// all or only what changed since the last review at recorded, and why.
func noteReviewScope(w io.Writer, full bool, recorded string, r git.ReviewDiff) {
	short := func(sha string) string { return sha[:min(7, len(sha))] }
	switch {
	case r.Since != "" && r.Diff == "" && len(r.Commits) > 0:
		_, _ = fmt.Fprintf(w, "bark: the %d commit(s) since the last review at %s change only files Bark ignores, so there's nothing to review (--full reviews the whole pull request)\n", len(r.Commits), short(r.Since))
	case r.Since != "" && r.Diff == "":
		_, _ = fmt.Fprintf(w, "bark: nothing changed since the last review at %s, so there's nothing to review (--full reviews the whole pull request)\n", short(r.Since))
	case r.Since != "":
		_, _ = fmt.Fprintf(w, "bark: reviewing only the changes since the last review at %s (%d commit(s))\n", short(r.Since), len(r.Commits))
	case full:
		_, _ = fmt.Fprintln(w, "bark: full review (--full)")
	case recorded == "":
		_, _ = fmt.Fprintln(w, "bark: full review: no earlier Bark review of this pull request recorded the commit it reviewed")
	case r.FullReason != "":
		_, _ = fmt.Fprintf(w, "bark: full review: %s\n", r.FullReason)
	}
}

// resolveReviewer loads a reviewer from a file path, by name, or from .bark/reviewer.md.
func resolveReviewer(name, storage string) (*reviewers.Reviewer, error) {
	if name != "" {
		if _, err := os.Stat(name); err == nil {
			return reviewers.FromFile(name)
		}
		reviewerList, err := reviewers.Get(storage)
		if err != nil {
			return nil, fmt.Errorf("error loading reviewers: %w", err)
		}
		r, err := reviewers.Find(name, reviewerList)
		if err != nil {
			return nil, fmt.Errorf("reviewer not found: %w", err)
		}
		return r, nil
	}

	if _, err := os.Stat(".bark/reviewer.md"); err == nil {
		return reviewers.FromFile(".bark/reviewer.md")
	}

	return nil, fmt.Errorf("reviewer is required in plain mode (use --as or add .bark/reviewer.md)")
}

// resolveInstructions returns the review instructions given by the
// --instructions values (see instructions.Resolve), or the repo's
// .bark/review.md when there are none.
func resolveInstructions(values []string, storage string) (string, error) {
	if len(values) > 0 {
		saved, _ := instructions.Get(storage)
		return instructions.Resolve(values, saved)
	}

	return utils.ReadLocalOverride(".bark/review.md")
}

// resolvePRInstructions returns the PR instructions given by the
// --instructions values (file paths or raw text), or the repo's .bark/pr.md
// or configured ones when there are none.
func resolvePRInstructions(values []string, cfg config.Config) (string, error) {
	if len(values) > 0 {
		return instructions.Resolve(values, nil)
	}

	return utils.GetInstructions(".bark/pr.md", cfg.GetPRInstructions())
}

// newMeteredClient creates the configured model's client, counting what its
// calls cost when there are prices for the model (see meterCalls).
func newMeteredClient(cfg config.Config) (llm.LLM, *prices.Meter, error) {
	client, provider, err := llm_factory.New(context.Background(), cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating LLM client: %w", err)
	}
	p, err := config.Paths()
	if err != nil {
		return client, nil, nil
	}
	model, _ := cfg.GetLLMModel()
	client, meter := meterCalls(client, p.Cache, provider, model)
	return client, meter, nil
}

// meterCalls wraps client to count what its calls cost, if the repo's or
// cacheDir's prices file lists the model. Otherwise it returns client and a
// nil Meter (which does nothing). A prices file that can't be read, or
// doesn't list the model, is a warning: the command goes on without costs.
func meterCalls(client llm.LLM, cacheDir, provider, model string) (llm.LLM, *prices.Meter) {
	pricing, err := prices.For(cacheDir, provider, model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bark: %v; costs won't be shown\n", err)
		return client, nil
	}
	if pricing == nil {
		return client, nil
	}
	meter := prices.NewMeter(client, pricing)
	return meter, meter
}

// printCost writes what the calls cost to stderr, if they were priced.
func printCost(meter *prices.Meter) {
	if s := meter.Summary(); s != "" {
		fmt.Fprintf(os.Stderr, "bark: %s\n", s)
	}
}

// warnTruncated tells the user on stderr, which stays out of pipes, that the
// model cut what off at its output limit.
func warnTruncated(what, consequence string) {
	fmt.Fprintf(os.Stderr, "bark: warning: the model reached its output limit, so the %s was cut off; %s\n", what, consequence)
}

// partialReviewNote returns the note that starts a review of a diff cut to
// fit its limit, or "" if cut is nil.
// warnReviewDiff writes the warnings about the diff to w (stderr).
func warnReviewDiff(w io.Writer, r git.ReviewDiff) {
	for _, warning := range r.Warnings {
		_, _ = fmt.Fprintf(w, "bark: warning: %s\n", warning)
	}
}

func partialReviewNote(cut *git.DiffCut) string {
	if cut == nil {
		return ""
	}
	return findings.PartialReviewNote(cut.Summary())
}

// withPartialNote returns events with the note about the cut diff first, or
// events as they are if cut is nil.
func withPartialNote(events []findings.Event, cut *git.DiffCut) []findings.Event {
	note := partialReviewNote(cut)
	if note == "" {
		return events
	}
	return append([]findings.Event{{Text: note}}, events...)
}
