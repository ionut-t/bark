package findings

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// The notes MarkedMarkdown writes under a set-aside finding's heading start
// with these; UncheckedNote is under a finding Bark's check didn't check,
// in Markdown too, as it isn't set aside.
const (
	DoubtedNote      = "> **Doubted by Bark's check:**"
	RaisedBeforeNote = "> **Raised before on this pull request:**"
	DismissedNote    = "> **Dismissed on this pull request:**"
	UncheckedNote    = "> **Not checked by Bark's check:**"
)

// MarkUnchecked returns events with every finding marked Unchecked for
// reason, for when Bark's check failed. The events passed in are left
// alone.
func MarkUnchecked(events []Event, reason string) []Event {
	out := slices.Clone(events)
	for i, e := range out {
		if e.Finding != nil {
			f := *e.Finding
			f.Unchecked = reason
			out[i].Finding = &f
		}
	}
	return out
}

// Markdown renders events as a Markdown review. Rendering events one batch
// at a time and joining the results gives the same output as rendering them
// all at once, so a streamed review can be appended to as it arrives.
//
// Findings Bark's check set aside (doubted, or raised before on the pull
// request) are moved to sections of their own before the verdict. Only a
// finished review has been checked, so this doesn't affect streaming.
func Markdown(events []Event) string {
	return markdown(events, false)
}

// MarkedMarkdown renders events as Markdown like Markdown, but keeps the
// findings Bark's check set aside where they are, each with a note under its
// heading saying why. A review already shown can be replaced by it without
// anything moving.
func MarkedMarkdown(events []Event) string {
	return markdown(events, true)
}

func markdown(events []Event, inPlace bool) string {
	var b strings.Builder
	var doubted, repeats, dismissed []Finding
	writeSetAside := func() {
		if len(doubted) > 0 {
			b.WriteString("## Doubted by Bark's check\n\n" +
				"A second pass checked these findings against the code and doubts them. " +
				"They are listed in case it's wrong.\n\n")
			b.WriteString(DoubtedList(doubted))
			doubted = nil
		}
		if len(repeats) > 0 {
			b.WriteString("## Raised before on this pull request\n\n" +
				"An earlier review of this pull request already raised these, so they aren't repeated as findings.\n\n")
			b.WriteString(RaisedBeforeList(repeats))
			repeats = nil
		}
		if len(dismissed) > 0 {
			b.WriteString("## Dismissed on this pull request\n\n" +
				"Someone with write access dismissed these points on an earlier review (👎 or \"won't fix\"), " +
				"so they aren't repeated as findings.\n\n")
			b.WriteString(DismissedList(dismissed))
			dismissed = nil
		}
	}
	for _, e := range events {
		switch {
		case e.Intro != "":
			fmt.Fprintf(&b, "%s\n\n", e.Intro)
		case e.Finding != nil && inPlace:
			writeFinding(&b, *e.Finding, "")
		case e.Finding != nil && e.Finding.Dismissed != "":
			dismissed = append(dismissed, *e.Finding)
		case e.Finding != nil && e.Finding.RaisedBefore != "":
			repeats = append(repeats, *e.Finding)
		case e.Finding != nil && e.Finding.Doubt != "":
			doubted = append(doubted, *e.Finding)
		case e.Finding != nil:
			writeFinding(&b, *e.Finding, "")
		case e.Verdict != "":
			writeSetAside()
			fmt.Fprintf(&b, "## Verdict\n\n%s\n\n", e.Verdict)
		default:
			b.WriteString(e.Text)
			b.WriteByte('\n')
		}
	}
	writeSetAside()
	return b.String()
}

// DoubtedList renders doubted findings as a Markdown list: each finding's
// heading and location, and why the check doubted it.
func DoubtedList(fs []Finding) string {
	return setAsideList(fs, func(f Finding) string { return f.Doubt })
}

// RaisedBeforeList renders findings that repeat a point raised before as a
// Markdown list: each finding's heading and location, and the earlier point.
func RaisedBeforeList(fs []Finding) string {
	return setAsideList(fs, func(f Finding) string { return "Same point as: " + f.RaisedBefore })
}

// DismissedList renders findings that repeat a point dismissed on the pull
// request as a Markdown list: each finding's heading and location, and the
// dismissed point.
func DismissedList(fs []Finding) string {
	return setAsideList(fs, func(f Finding) string { return "Dismissed point: " + f.Dismissed })
}

func setAsideList(fs []Finding, why func(Finding) string) string {
	var b strings.Builder
	for _, f := range fs {
		fmt.Fprintf(&b, "- **%s**", heading(f))
		if loc := location(f); loc != "" {
			fmt.Fprintf(&b, " `%s`", loc)
		}
		fmt.Fprintf(&b, "\n  %s\n", oneLine(why(f)))
	}
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	return b.String()
}

// TruncatedNote ends a review the model cut off at its output limit.
const TruncatedNote = "> **Cut off:** the model reached its output limit, so this review ends early and may be missing findings.\n"

// PartialReviewNote starts a review of a diff that was cut to fit its
// limit; summary says how much of it was reviewed.
func PartialReviewNote(summary string) string {
	return "> **Partial review:** " + summary + "\n"
}

// CheckReport renders the outcome of Bark's check for a review that has
// already been shown, so it can be appended to it: how many findings were
// checked, which were doubted and which were raised before.
func CheckReport(events []Event, checked int) string {
	var doubted, repeats, dismissed []Finding
	for _, e := range events {
		switch {
		case e.Finding == nil:
		case e.Finding.Dismissed != "":
			dismissed = append(dismissed, *e.Finding)
		case e.Finding.RaisedBefore != "":
			repeats = append(repeats, *e.Finding)
		case e.Finding.Doubt != "":
			doubted = append(doubted, *e.Finding)
		}
	}
	if checked == 0 {
		return ""
	}
	if len(doubted) == 0 && len(repeats) == 0 && len(dismissed) == 0 {
		return fmt.Sprintf("## Bark's check\n\nA second pass checked %s against the code and kept %s.\n\n",
			plural(checked, "finding"), them(checked))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## Bark's check\n\nA second pass checked %s against the code", plural(checked, "finding"))
	if len(doubted) > 0 {
		fmt.Fprintf(&b, " and doubts %d:\n\n%s", len(doubted), DoubtedList(doubted))
	} else {
		b.WriteString(". ")
	}
	if len(repeats) > 0 {
		fmt.Fprintf(&b, "%d %s raised before on this pull request:\n\n%s", len(repeats), wasWere(len(repeats)), RaisedBeforeList(repeats))
	}
	if len(dismissed) > 0 {
		fmt.Fprintf(&b, "%d %s dismissed on this pull request:\n\n%s", len(dismissed), wasWere(len(dismissed)), DismissedList(dismissed))
	}
	return b.String()
}

// oneLine joins s onto one line, so it fits in a list item or a quote.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func wasWere(n int) string {
	if n == 1 {
		return "was"
	}
	return "were"
}

func them(n int) string {
	if n == 1 {
		return "it"
	}
	return "them all"
}

// writeFinding writes f as Markdown. open names a prose field (title, body
// or remark) whose value is still streaming in: writing stops right after
// that value, so the text written is a prefix of the finished finding's.
func writeFinding(b *strings.Builder, f Finding, open string) {
	fmt.Fprintf(b, "### %s", heading(f))
	if open == "title" {
		return
	}
	b.WriteString("\n\n")

	switch {
	case f.Dismissed != "":
		fmt.Fprintf(b, "%s %s\n\n", DismissedNote, oneLine(f.Dismissed))
	case f.RaisedBefore != "":
		fmt.Fprintf(b, "%s %s\n\n", RaisedBeforeNote, oneLine(f.RaisedBefore))
	case f.Doubt != "":
		fmt.Fprintf(b, "%s %s\n\n", DoubtedNote, oneLine(f.Doubt))
	case f.Unchecked != "":
		fmt.Fprintf(b, "%s %s\n\n", UncheckedNote, oneLine(f.Unchecked))
	}

	if loc := location(f); loc != "" {
		fmt.Fprintf(b, "`%s`\n\n", loc)
	}

	if f.Body != "" {
		b.WriteString(f.Body)
		if open == "body" {
			return
		}
		b.WriteString("\n\n")
	}

	if f.Suggestion != "" {
		fence := "```"
		for strings.Contains(f.Suggestion, fence) {
			fence += "`"
		}
		fmt.Fprintf(b, "%s%s\n%s\n%s\n\n", fence, language(f.File), strings.TrimRight(f.Suggestion, "\n"), fence)
	}

	if f.Remark != "" {
		b.WriteString("> _" + f.Remark)
		if open == "remark" {
			return
		}
		b.WriteString("_\n\n")
	}
}

// heading is "[SEVERITY · category] Title".
func heading(f Finding) string {
	h := strings.ToUpper(string(f.Severity))
	if f.Category != "" {
		h += " · " + f.Category
	}
	return fmt.Sprintf("[%s] %s", h, f.Title)
}

func location(f Finding) string {
	switch {
	case f.File == "":
		return ""
	case f.Line <= 0:
		return f.File
	case f.EndLine > f.Line:
		return fmt.Sprintf("%s:%d-%d", f.File, f.Line, f.EndLine)
	default:
		return fmt.Sprintf("%s:%d", f.File, f.Line)
	}
}

var languages = map[string]string{
	".go":    "go",
	".py":    "python",
	".js":    "javascript",
	".jsx":   "jsx",
	".ts":    "typescript",
	".tsx":   "tsx",
	".rs":    "rust",
	".java":  "java",
	".kt":    "kotlin",
	".swift": "swift",
	".rb":    "ruby",
	".php":   "php",
	".c":     "c",
	".h":     "c",
	".cpp":   "cpp",
	".cc":    "cpp",
	".hpp":   "cpp",
	".cs":    "csharp",
	".sh":    "bash",
	".sql":   "sql",
	".yaml":  "yaml",
	".yml":   "yaml",
	".json":  "json",
	".toml":  "toml",
	".md":    "markdown",
	".html":  "html",
	".css":   "css",
}

// language is the code fence language for file, or "text" if unknown.
func language(file string) string {
	if lang, ok := languages[strings.ToLower(filepath.Ext(file))]; ok {
		return lang
	}
	return "text"
}

// WarningNote starts a review whose diff came with a warning, e.g. a pull
// request whose metadata couldn't be read.
func WarningNote(warning string) string {
	return "> **Warning:** " + warning + "\n"
}
