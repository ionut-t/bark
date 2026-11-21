package findings

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSeverityAtLeast(t *testing.T) {
	t.Parallel()

	tests := []struct {
		s, min Severity
		want   bool
	}{
		{Critical, High, true},
		{High, High, true},
		{Medium, High, false},
		{Low, Medium, false},
		{Low, Low, true},
		{"", Critical, false},
		{Low, "", true},
		// Values outside the scale count as medium.
		{"nuclear", Medium, true},
		{"nuclear", High, false},
		{Critical, "nuclear", true},
		{Low, "nuclear", false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.s.AtLeast(tt.min), "%q at least %q", tt.s, tt.min)
	}
}

func TestDropBelow(t *testing.T) {
	t.Parallel()

	events := []Event{
		{Intro: "Hello."},
		{Finding: &Finding{Title: "crit", Severity: Critical}},
		{Finding: &Finding{Title: "low", Severity: Low}},
		{Text: "stray"},
		{Finding: &Finding{Title: "odd", Severity: "nuclear"}},
		{Finding: &Finding{Title: "high", Severity: High}},
		{Verdict: "Bye."},
	}

	kept, dropped := DropBelow(events, High)
	assert.Equal(t, 2, dropped)
	var titles []string
	for _, e := range kept {
		if e.Finding != nil {
			titles = append(titles, e.Finding.Title)
		}
	}
	assert.Equal(t, []string{"crit", "high"}, titles)
	assert.Len(t, kept, 5, "intro, text and verdict are kept")

	all, dropped := DropBelow(events, "")
	assert.Zero(t, dropped)
	assert.Equal(t, events, all)
}

func TestRendererLeavesOutFindingsBelowMin(t *testing.T) {
	t.Parallel()

	output := `{"type":"intro","text":"Hi."}` + "\n" +
		`{"file":"a.go","line":1,"severity":"low","category":"style","title":"Low one","body":"meh"}` + "\n" +
		`{"file":"a.go","line":2,"severity":"high","category":"bug","title":"High one","body":"bad"}` + "\n" +
		`{"type":"verdict","text":"Done."}`

	kept, _ := DropBelow(Parse(output), High)
	want := Markdown(kept)

	for size := 1; size <= len(output); size++ {
		r := Renderer{Min: High}
		var b strings.Builder
		for i := 0; i < len(output); i += size {
			b.WriteString(r.Write(output[i:min(i+size, len(output))]))
		}
		b.WriteString(r.Close())
		got := b.String()
		assert.Equal(t, want, got, "chunk size %d", size)
		assert.NotContains(t, got, "Low one")
	}
}

// Blocking counts findings at or above the threshold that the check didn't
// doubt. A repeat of a point raised before counts: the model found it again
// in the code, so it's still there.
func TestBlocking(t *testing.T) {
	t.Parallel()

	events := []Event{
		{Intro: "Hi."},
		{Finding: &Finding{File: "a.go", Line: 1, Severity: Critical, Title: "Kept critical"}},
		{Finding: &Finding{File: "b.go", Line: 2, Severity: High, Title: "Doubted high", Doubt: "Not so."}},
		{Finding: &Finding{File: "c.go", Line: 3, Severity: "High", Title: "Kept high"}},
		{Finding: &Finding{File: "d.go", Line: 4, Severity: Critical, Title: "Repeat", RaisedBefore: "Nil map"}},
		{Finding: &Finding{File: "e.go", Line: 5, Severity: Medium, Title: "Kept medium"}},
		{Finding: &Finding{File: "f.go", Line: 6, Severity: High, Title: "Unchecked high", Unchecked: "it failed."}},
		{Verdict: "Done."},
	}

	titles := func(fs []Finding) []string {
		var out []string
		for _, f := range fs {
			out = append(out, f.Title)
		}
		return out
	}

	assert.Equal(t, []string{"Kept critical", "Kept high", "Repeat", "Unchecked high"}, titles(Blocking(events, High)))
	assert.Equal(t, []string{"Kept critical", "Repeat"}, titles(Blocking(events, Critical)))
	assert.Equal(t, []string{"Kept critical", "Kept high", "Repeat", "Kept medium", "Unchecked high"}, titles(Blocking(events, Low)))
	assert.Empty(t, Blocking(events, ""), "no threshold blocks nothing")
}
