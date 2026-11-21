package check

import (
	"fmt"
	"strings"

	"github.com/ionut-t/bark/v2/internal/findings"
)

// Thread is an open thread of one of Bark's inline comments from an earlier
// review of the pull request, whose code changed since. The check says
// whether the point it raised is fixed; Bark resolves it if so.
type Thread struct {
	// Point is what the comment raised: its file, title and body, and the
	// lines it covered (Line to EndLine) on Commit, the commit it was
	// posted on.
	Point findings.Finding
	// Line and EndLine are where those lines, or the lines that replaced
	// them, are in the new version of the file; 0 when nothing is left of
	// them.
	Line, EndLine int
	// Before is the file at Point.Commit. A thread is found fixed only
	// with evidence that wasn't in it.
	Before string
}

// at is where t's code is now, as a finding, so the check is shown the
// lines around it.
func (t Thread) at() findings.Finding {
	return findings.Finding{File: t.Point.File, Line: t.Line, EndLine: t.EndLine}
}

// shownFor is what the check is shown files for: the findings, and where
// the code of the open threads is now.
func shownFor(fs []findings.Finding, threads []Thread) []findings.Finding {
	if len(threads) == 0 {
		return fs
	}
	out := append([]findings.Finding(nil), fs...)
	for _, t := range threads {
		out = append(out, t.at())
	}
	return out
}

// writeThreads adds the open threads to the prompt: each point, the code
// it was about then, and where that code is now.
func writeThreads(b *strings.Builder, threads []Thread) {
	if len(threads) == 0 {
		return
	}
	b.WriteString("\n## Open threads\n\n" +
		"Earlier reviews of this pull request posted these points as inline comments, which are still open. " +
		"The code each was about has changed since. Line numbers under \"Then\" are from the commit the point was raised on.\n\n")
	for i, t := range threads {
		p := t.Point
		fmt.Fprintf(b, "%d. `%s` %s: %s\n", i+1, location(p), oneLine(p.Title), oneLine(p.Body))
		if old := linesOf(t.Before, p.Line, max(p.EndLine, p.Line)); old != "" {
			fence := "```"
			for strings.Contains(old, fence) {
				fence += "`"
			}
			fmt.Fprintf(b, "   Then, at `%s`:\n   %s\n", p.Commit[:min(7, len(p.Commit))], fence)
			for l := range strings.SplitSeq(strings.TrimSuffix(old, "\n"), "\n") {
				fmt.Fprintf(b, "   %s\n", l)
			}
			fmt.Fprintf(b, "   %s\n", fence)
		}
		switch {
		case t.Line <= 0:
			b.WriteString("   Now: nothing is left of those lines.\n")
		case t.EndLine > t.Line:
			fmt.Fprintf(b, "   Now: lines %d-%d of the new %s.\n", t.Line, t.EndLine, p.File)
		default:
			fmt.Fprintf(b, "   Now: line %d of the new %s.\n", t.Line, p.File)
		}
	}
}

// linesOf returns lines start..end of file, each after its number and a
// tab, or "" if file has none of them.
func linesOf(file string, start, end int) string {
	if file == "" || start <= 0 {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(file, "\n"), "\n")
	var b strings.Builder
	for n := start; n <= min(end, len(lines)); n++ {
		fmt.Fprintf(&b, "%d\t%s\n", n, lines[n-1])
	}
	return b.String()
}

// parseThreadVerdicts reads the check's verdicts on open threads from its
// JSON Lines output, by thread number. A later verdict for the same
// thread replaces an earlier one.
func parseThreadVerdicts(output string) map[int]Verdict {
	out := map[int]Verdict{}
	eachVerdict(output, func(v Verdict) {
		if v.Thread > 0 {
			out[v.Thread] = v
		}
	})
	return out
}

// applyThreads returns the open threads of in that the verdicts find fixed,
// by index, with the check's reason, and why each other thread stays open.
// A thread is fixed only with a reason, and with evidence quoted from the
// new code the check was shown (the files, around fs and the threads, and
// the related code) of which at least one line wasn't in the file when the
// point was raised: the code that fixes it, not code that was already
// there.
func applyThreads(in Input, fs []findings.Finding, verdicts map[int]Verdict) (fixed, open map[int]string) {
	if len(in.Threads) == 0 {
		return nil, nil
	}
	now := newCorpus("", excerpts(in.Files, shownFor(fs, in.Threads)), in.Related)
	fixed, open = map[int]string{}, map[int]string{}
	for i, t := range in.Threads {
		v, ok := verdicts[i+1]
		reason := strings.TrimSpace(v.Reason)
		switch {
		case !ok:
			open[i] = "the check gave no verdict"
		case v.Verdict != "fixed":
			if reason == "" {
				reason = "the check's verdict was " + v.Verdict
			}
			open[i] = reason
		case reason == "":
			open[i] = "the check called it fixed without a reason"
		case t.Before == "":
			open[i] = "the check called it fixed, but Bark couldn't read the code it was about then"
		case !now.contains(v.Evidence):
			open[i] = "the check called it fixed, but the code it quoted isn't in the code now"
		case !newLine(v.Evidence, t.Before):
			open[i] = "the check called it fixed, but the code it quoted was already there when the point was raised"
		default:
			fixed[i] = reason
		}
	}
	return orNil(fixed), orNil(open)
}

// orNil returns m, or nil when it's empty.
func orNil(m map[int]string) map[int]string {
	if len(m) == 0 {
		return nil
	}
	return m
}

// withoutRepeated moves the fixed threads whose point a finding was
// dropped as a repeat of, by the verdicts, to open: the check can't find
// the point both fixed and raised again, so those threads stay open.
func withoutRepeated(fixed, open map[int]string, in Input, fs []findings.Finding, verdicts map[int]Verdict) (map[int]string, map[int]string) {
	if len(fixed) == 0 {
		return fixed, open
	}
	points := priorPoints(in.Prior)
	repeated := map[string]bool{}
	for i, f := range fs {
		if v, ok := verdicts[i+1]; ok && v.Verdict != "keep" {
			if p, ok := raisedBefore(v.Reason, f, points); ok {
				repeated[priorKey(p)] = true
			}
		}
	}
	for i := range fixed {
		if repeated[priorKey(in.Threads[i].Point)] {
			delete(fixed, i)
			if open == nil {
				open = map[int]string{}
			}
			open[i] = "the check called it fixed, but also set aside a new finding as a repeat of its point"
		}
	}
	return orNil(fixed), open
}

// newLine reports whether a line of evidence long enough to count isn't
// part of any line of before.
func newLine(evidence, before string) bool {
	var old corpus
	for l := range strings.SplitSeq(before, "\n") {
		old.add(l)
	}
	for l := range strings.SplitSeq(evidence, "\n") {
		l = normalise(stripEvidenceLine(l))
		if len(l) >= minEvidence && !elided(l) && !old.has(l) {
			return true
		}
	}
	return false
}
