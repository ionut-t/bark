package tui

import (
	"math/rand"
	"time"

	tea "charm.land/bubbletea/v2"
)

// loadingMessagePicker shows the messages in a random order, each once per
// round, and never the same one twice in a row.
type loadingMessagePicker struct {
	messages []string
	// order is what is left of the current round, as indexes into messages.
	order []int
	last  int
}

func newLoadingMessagePicker(messages []string) *loadingMessagePicker {
	return &loadingMessagePicker{messages: messages, last: -1}
}

func (p *loadingMessagePicker) next() string {
	if len(p.messages) == 0 {
		return "Loading..."
	}
	if len(p.order) == 0 {
		p.order = rand.Perm(len(p.messages))
		// A new round doesn't start with the message the last one ended on.
		if len(p.order) > 1 && p.order[0] == p.last {
			p.order[0], p.order[1] = p.order[1], p.order[0]
		}
	}
	p.last, p.order = p.order[0], p.order[1:]
	return p.messages[p.last]
}

func dispatchLoadingMessage(msg tea.Msg) tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return msg
	})
}
