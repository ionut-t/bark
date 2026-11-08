// Package prreview turns structured findings into a GitHub pull request
// review: findings on lines the diff shows become inline comments, with a
// suggestion block GitHub can apply in one click; everything else (the
// persona's intro and verdict, and findings GitHub can't attach to a line)
// goes in the review body.
package prreview

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/ionut-t/bark/v2/internal/findings"
)

// Marker starts the body of every review Bark posts, so it can be found
// again.
const Marker = "<!-- bark-review -->"

// Review is the request body of GitHub's "create a review for a pull
// request" endpoint.
type Review struct {
	CommitID string    `json:"commit_id,omitempty"`
	Body     string    `json:"body"`
	Event    string    `json:"event"`
	Comments []Comment `json:"comments"`
}

// Comment is an inline review comment on the new side of the diff. Line is
// the last line it covers; StartLine is set when it covers several.
type Comment struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Side      string `json:"side"`
	StartLine int    `json:"start_line,omitempty"`
	StartSide string `json:"start_side,omitempty"`
	Body      string `json:"body"`
}

// Build turns the events of a structured review of diff into a Review.
// A finding becomes an inline comment when its line is an added or context
// line of diff; GitHub rejects comments on any other line. With an empty
// diff, every finding goes in the body.
//
// A suggestion becomes a suggestion block, which GitHub applies in one
// click, only when the finding's original text is found on the lines the
// comment covers. Models sometimes get a line number wrong by one or two, so
// the comment moves to the match nearest the line the model gave. Without a
// match, the suggestion is shown as plain code.
func Build(events []findings.Event, diff string) Review {
	lines := commentableLines(diff)

	var comments []Comment
	var rest []findings.Event
	var doubted []findings.Finding
	for _, e := range events {
		if e.Finding != nil && e.Finding.Doubt != "" {
			doubted = append(doubted, *e.Finding)
			continue
		}
		if e.Finding != nil {
			if c, ok := comment(*e.Finding, lines[e.Finding.File]); ok {
				comments = append(comments, c)
				continue
			}
		}
		if strings.HasPrefix(strings.TrimSpace(e.Text), "{") {
			e.Text = folded(e.Text)
		}
		rest = append(rest, e)
	}
	rest = withDoubted(rest, doubted)

	body := Marker + "\n## Bark AI Code Review\n\n" + strings.TrimSpace(findings.Markdown(rest))
	return Review{
		Body:     strings.TrimSpace(body) + "\n",
		Event:    "COMMENT",
		Comments: comments,
	}
}

// withDoubted puts the findings Bark's check doubted in a collapsed block
// before the verdict (or at the end), so they don't get inline comments but
// can still be read if the check was wrong.
func withDoubted(events []findings.Event, doubted []findings.Finding) []findings.Event {
	if len(doubted) == 0 {
		return events
	}
	block := findings.Event{Text: fmt.Sprintf("<details>\n<summary>Bark's check doubted %d finding(s)</summary>\n\n"+
		"A second pass checked these against the code and doubts them.\n\n%s</details>\n",
		len(doubted), findings.DoubtedList(doubted))}

	i := slices.IndexFunc(events, func(e findings.Event) bool { return e.Verdict != "" })
	if i < 0 {
		return append(events, block)
	}
	return slices.Insert(events, i, block)
}

// folded wraps a line that looks like a finding but isn't valid JSON
// (models sometimes leave quotes in copied code unescaped) in a collapsed
// code block, so it doesn't fill the review body with raw JSON.
func folded(line string) string {
	fence := "```"
	for strings.Contains(line, fence) {
		fence += "`"
	}
	return fmt.Sprintf("<details>\n<summary>A finding Bark couldn't read</summary>\n\n%sjson\n%s\n%s\n\n</details>\n",
		fence, strings.TrimSpace(line), fence)
}

func comment(f findings.Finding, lines map[int]diffLine) (Comment, bool) {
	start, end, verified := locate(f, lines)
	if start == 0 {
		return Comment{}, false
	}

	// A repaired finding's suggestion may have been cut short, so it is
	// never offered as one to apply.
	verified = verified && !f.Repaired
	if verified {
		file := make([]string, 0, end-start+1)
		for l := start; l <= end; l++ {
			file = append(file, lines[l].text)
		}
		f.Suggestion, verified = fixIndent(splitLines(f.Original), file, f.Suggestion)
	}

	c := Comment{Path: f.File, Line: end, Side: "RIGHT", Body: commentBody(f, verified)}
	if end > start {
		c.StartLine, c.StartSide = start, "RIGHT"
	}
	return c, true
}

// locate returns the lines a comment on f covers, or 0s if it can't go on
// a line, and whether they hold f's original text.
func locate(f findings.Finding, lines map[int]diffLine) (start, end int, verified bool) {
	if f.Suggestion != "" && f.Original != "" {
		want := splitLines(f.Original)
		best := 0
		for l := range lines {
			if matches(lines, l, want) && (best == 0 || closer(l, best, f.Line)) {
				best = l
			}
		}
		if best != 0 {
			return best, best + len(want) - 1, true
		}
	}

	if _, ok := lines[f.Line]; f.Line <= 0 || !ok {
		return 0, 0, false
	}
	if f.EndLine > f.Line && inOneHunk(lines, f.Line, f.EndLine) {
		return f.Line, f.EndLine, false
	}
	return f.Line, f.Line, false
}

// matches reports whether the lines from start hold want, in one hunk.
// Leading and trailing whitespace is ignored: models often drop
// indentation when they copy code (fixIndent deals with that).
func matches(lines map[int]diffLine, start int, want []string) bool {
	if !inOneHunk(lines, start, start+len(want)-1) {
		return false
	}
	for i, w := range want {
		if strings.TrimSpace(lines[start+i].text) != strings.TrimSpace(w) {
			return false
		}
	}
	return true
}

// fixIndent returns suggestion with the indentation the model dropped when
// it copied original from file put back. Models often drop the leading
// whitespace of the first line, or of every line, of both original and
// suggestion. ok is false when the indentation can't be worked out; the
// suggestion then mustn't be offered as one to apply.
func fixIndent(original, file []string, suggestion string) (string, bool) {
	// added is the whitespace missing from the start of each line of
	// original, or "" when it's complete or the line is blank.
	added := make([]string, len(original))
	var lastAdded string
	nonBlank, missing := 0, 0
	for i, o := range original {
		if strings.TrimSpace(o) == "" {
			continue
		}
		fi, oi := indent(file[i]), indent(o)
		if !strings.HasSuffix(fi, oi) {
			return suggestion, false
		}
		added[i] = fi[:len(fi)-len(oi)]
		nonBlank++
		if added[i] != "" {
			missing++
			if lastAdded != "" && added[i] != lastAdded {
				return suggestion, false
			}
			lastAdded = added[i]
		}
	}
	if missing == 0 {
		return suggestion, true
	}

	sug := splitLines(suggestion)
	switch indent(sug[0]) {
	case indent(file[0]):
		// The suggestion kept its indentation; only original lost it.
		return suggestion, true
	case strings.TrimPrefix(indent(file[0]), added[0]):
		// The suggestion lost the same indentation as original.
	default:
		return suggestion, false
	}

	sugNonBlank := 0
	for _, l := range sug {
		if strings.TrimSpace(l) != "" {
			sugNonBlank++
		}
	}

	var toFirstOnly bool
	switch {
	case missing == nonBlank && (nonBlank > 1 || sugNonBlank == 1):
		// Every line lost the same indentation.
	case missing == 1 && added[0] != "" && nonBlank > 1:
		// Only the first line lost its indentation.
		toFirstOnly = true
	default:
		// One line of original lost its indentation but the suggestion
		// has several: whether the rest of the suggestion lost it too
		// can't be told.
		return suggestion, false
	}

	for i, l := range sug {
		if strings.TrimSpace(l) != "" {
			sug[i] = lastAdded + l
		}
		if toFirstOnly {
			break
		}
	}
	return strings.Join(sug, "\n"), true
}

func indent(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " \t"))]
}

func splitLines(s string) []string {
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

func inOneHunk(lines map[int]diffLine, start, end int) bool {
	first, ok := lines[start]
	if !ok {
		return false
	}
	for l := start + 1; l <= end; l++ {
		if d, ok := lines[l]; !ok || d.hunk != first.hunk {
			return false
		}
	}
	return true
}

// closer reports whether a is nearer to target than b, preferring the
// earlier line on a tie.
func closer(a, b, target int) bool {
	da, db := abs(a-target), abs(b-target)
	return da < db || da == db && a < b
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// commentBody renders f for an inline comment. Its location is the line
// the comment is on, so it isn't repeated. The suggestion becomes a
// suggestion block only when the comment covers exactly the lines it
// replaces.
func commentBody(f findings.Finding, applicable bool) string {
	var b strings.Builder

	heading := strings.ToUpper(string(f.Severity))
	if f.Category != "" {
		heading += " · " + f.Category
	}
	fmt.Fprintf(&b, "**[%s] %s**\n\n", heading, f.Title)

	if f.Body != "" {
		b.WriteString(f.Body)
		b.WriteString("\n\n")
	}

	if f.Suggestion != "" {
		fence := "```"
		for strings.Contains(f.Suggestion, fence) {
			fence += "`"
		}
		info := ""
		if applicable {
			info = "suggestion"
		}
		fmt.Fprintf(&b, "%s%s\n%s\n%s\n\n", fence, info, strings.TrimRight(f.Suggestion, "\n"), fence)
	}

	if f.Remark != "" {
		fmt.Fprintf(&b, "> _%s_\n\n", f.Remark)
	}

	return strings.TrimRight(b.String(), "\n")
}

var (
	newFileHeader = regexp.MustCompile(`^\+\+\+ (?:b/)?(.+?)\t?$`)
	hunkHeader    = regexp.MustCompile(`^@@ -\d+(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)
)

// diffLine is an added or context line of a diff: its text without the
// diff marker, and the index of the hunk it's in.
type diffLine struct {
	hunk int
	text string
}

// commentableLines maps each file in diff to its added and context lines on
// the new side, by line number.
func commentableLines(diff string) map[string]map[int]diffLine {
	lines := make(map[string]map[int]diffLine)
	var file string
	var hunk, newLine, oldLeft, newLeft int

	for l := range strings.Lines(diff) {
		l = strings.TrimRight(l, "\r\n")

		if oldLeft == 0 && newLeft == 0 {
			if m := newFileHeader.FindStringSubmatch(l); m != nil {
				file = m[1]
				if file == "/dev/null" {
					file = ""
				}
				continue
			}
		}

		if m := hunkHeader.FindStringSubmatch(l); m != nil {
			hunk++
			oldLeft = count(m[1])
			newLine, _ = strconv.Atoi(m[2])
			newLeft = count(m[3])
			continue
		}

		if oldLeft == 0 && newLeft == 0 {
			continue
		}

		switch {
		case strings.HasPrefix(l, "-"):
			oldLeft--
		case strings.HasPrefix(l, "+"), strings.HasPrefix(l, " "), l == "":
			if file != "" {
				if lines[file] == nil {
					lines[file] = make(map[int]diffLine)
				}
				text := ""
				if l != "" {
					text = l[1:]
				}
				lines[file][newLine] = diffLine{hunk: hunk, text: text}
			}
			newLine++
			newLeft--
			if !strings.HasPrefix(l, "+") {
				oldLeft--
			}
		}
	}

	return lines
}

// count parses a hunk header's line count, which is 1 when omitted.
func count(s string) int {
	if s == "" {
		return 1
	}
	n, _ := strconv.Atoi(s)
	return n
}
