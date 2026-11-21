package prreview

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/ionut-t/bark/v2/internal/findings"
)

// recordStart opens the hidden block at the end of a posted review that
// lists its findings, so a later review of the same pull request knows what
// was raised (see Recorded). The block is an HTML comment, which GitHub
// doesn't show.
const recordStart = "<!-- bark-findings v1"

// recorded is one finding as kept in a review's hidden block: enough to
// tell whether a later finding makes the same point.
type recorded struct {
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	EndLine int    `json:"end_line,omitempty"`
	Title   string `json:"title"`
	Body    string `json:"body,omitempty"`
	// Dismissed was set on a finding set aside as a repeat of a point
	// dismissed on the pull request. Bark no longer records repeats (see
	// record), but still reads it from reviews that did.
	Dismissed string `json:"dismissed,omitempty"`
	// Doubted is true when Bark's check doubted the finding, so it wasn't
	// posted as a point to fix.
	Doubted bool `json:"doubted,omitempty"`
}

// reviewedCommits is the line of the hidden block that names the commit
// the next review can start from and the pull request's merge base then.
// Bark versions that don't know it skip it, as it has no file or title.
type reviewedCommits struct {
	Head string `json:"head"`
	Base string `json:"base,omitempty"`
}

// record renders the hidden block for every finding in events, posted or
// doubted, one JSON object per line, and the commits of scope the next
// review can start from, if any. It's empty without either.
//
// Findings set aside as repeats of a point raised before, or of a
// dismissed one, are left out: that point is already recorded (and its
// dismissal is read again from the pull request), while the model gives
// a repeat a new title on every re-run, so recording it would add a point
// each time and make the repeat harder for the check to match.
func record(events []findings.Event, scope Scope) string {
	var b strings.Builder
	if head := scope.recordedHead(); head != "" {
		line, _ := json.Marshal(reviewedCommits{Head: head, Base: scope.Base})
		b.WriteString(recordStart + "\n")
		b.Write(line)
		b.WriteByte('\n')
	}
	for _, e := range events {
		f := e.Finding
		if f == nil || f.RaisedBefore != "" || f.Dismissed != "" {
			continue
		}
		if b.Len() == 0 {
			b.WriteString(recordStart + "\n")
		}
		// json.Marshal escapes < and >, so no title or body can end the
		// comment early.
		line, err := json.Marshal(recorded{
			File: f.File, Line: f.Line, EndLine: f.EndLine, Title: f.Title,
			Body: findings.FirstSentence(f.Body), Doubted: f.Doubt != "",
		})
		if err != nil {
			continue
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if b.Len() > 0 {
		b.WriteString("-->\n")
	}
	return b.String()
}

// recordBlock returns the hidden block of a review body Bark posted, or ""
// if it has none.
func recordBlock(body string) string {
	// The block is the last thing in the body; a finding shown above it
	// could quote recordStart.
	i := strings.LastIndex(body, recordStart)
	if i < 0 {
		return ""
	}
	block, _, _ := strings.Cut(body[i+len(recordStart):], "-->")
	return block
}

var commitHash = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

// RecordedScope returns the commits the hidden block of the review with
// body records: the one the next review can start from (Head) and the pull
// request's merge base then (Base, if known). It's zero if the block
// records none, as in reviews posted before Bark recorded them.
func RecordedScope(body string) Scope {
	for line := range strings.SplitSeq(recordBlock(body), "\n") {
		var c reviewedCommits
		// They're read back into git commands: take only hashes.
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &c) == nil && commitHash.MatchString(c.Head) {
			s := Scope{Head: c.Head}
			if commitHash.MatchString(c.Base) {
				s.Base = c.Base
			}
			return s
		}
	}
	return Scope{}
}

// Recorded returns the findings listed in the hidden block of a review
// body Bark posted. Reviews posted before Bark recorded findings have none.
func Recorded(body string) []findings.Finding {
	var out []findings.Finding
	for line := range strings.SplitSeq(recordBlock(body), "\n") {
		line = strings.TrimSpace(line)
		var r recorded
		if line == "" || json.Unmarshal([]byte(line), &r) != nil || r.File == "" || r.Title == "" {
			continue
		}
		f := findings.Finding{
			File: r.File, Line: r.Line, EndLine: r.EndLine, Title: findings.CleanTitle(r.Title), Body: r.Body,
			Dismissed: r.Dismissed,
		}
		if r.Doubted {
			f.Doubt = findings.DoubtedEarlier
		}
		out = append(out, f)
	}
	return out
}
