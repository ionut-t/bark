package prreview

import (
	"testing"

	"github.com/ionut-t/bark/v2/internal/findings"
	"github.com/stretchr/testify/assert"
)

func TestNothingNew(t *testing.T) {
	t.Parallel()

	const head, base = "fc4cb814c9a4b110f6b03b5f77182099e9f1dfee", "378188b5e814fa4adfb81620464f29a85fa75a4a"
	last := History{Head: head, Base: base}
	repeat := findings.Event{Finding: &findings.Finding{File: "a.go", Line: 2, Title: "Again", RaisedBefore: "Earlier"}}
	dismissed := findings.Event{Finding: &findings.Finding{File: "a.go", Line: 3, Title: "Won't fix", Dismissed: "Earlier"}}
	text := []findings.Event{{Intro: "Hello."}, {Verdict: "Bye."}}
	with := func(es ...findings.Event) []findings.Event {
		return append(append([]findings.Event{text[0]}, es...), text[1])
	}

	tests := []struct {
		name   string
		events []findings.Event
		scope  Scope
		last   History
		want   bool
	}{
		{"only repeats, same head", with(repeat, dismissed), Scope{Head: head, Base: base}, last, true},
		{"no findings, same head", text, Scope{Head: head, Base: base}, last, true},
		{"merge base unknown now", with(repeat), Scope{Head: head}, last, true},
		{"merge base unknown then", with(repeat), Scope{Head: head, Base: base}, History{Head: head}, true},
		{"a new finding", with(repeat, findings.Event{Finding: &findings.Finding{File: "a.go", Line: 4, Title: "New"}}),
			Scope{Head: head, Base: base}, last, false},
		{"a doubted finding", with(repeat, findings.Event{Finding: &findings.Finding{File: "a.go", Line: 4, Title: "Hmm", Doubt: "No."}}),
			Scope{Head: head, Base: base}, last, false},
		{"head moved", with(repeat), Scope{Head: "0123456789abcdef0123456789abcdef01234567", Base: base}, last, false},
		{"merge base moved", with(repeat), Scope{Head: head, Base: "0123456789abcdef0123456789abcdef01234567"}, last, false},
		{"partial review", with(repeat), Scope{Head: head, Base: base, Partial: true}, last, false},
		{"no head now", with(repeat), Scope{}, last, false},
		{"no earlier review recorded a head", with(repeat), Scope{Head: head, Base: base}, History{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, NothingNew(tt.events, tt.scope, tt.last))
		})
	}
}
