package tui

import (
	"math/rand"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type loadingMessagePicker struct {
	messages     []string
	usedMessages map[int]struct{}
}

func newLoadingMessagePicker(messages []string) *loadingMessagePicker {
	return &loadingMessagePicker{
		messages:     messages,
		usedMessages: make(map[int]struct{}, len(messages)),
	}
}

func (p *loadingMessagePicker) next() string {
	if len(p.messages) == 0 {
		return "Loading..."
	}

	// Reset if we've used all messages
	if len(p.usedMessages) >= len(p.messages) {
		p.usedMessages = make(map[int]struct{}, len(p.messages))
	}

	// Try to find an unused message
	for attempts := 0; attempts < len(p.messages)*2; attempts++ {
		index := rand.Intn(len(p.messages))
		if _, used := p.usedMessages[index]; !used {
			p.usedMessages[index] = struct{}{}
			return p.messages[index]
		}
	}

	// Fallback
	return p.messages[0]
}

func dispatchLoadingMessage(msg tea.Msg) tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return msg
	})
}
