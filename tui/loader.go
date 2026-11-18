package tui

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ionut-t/coffee/styles"
)

// loader is a spinner with a loading message that changes every few
// seconds. Each Start gives it a new ID, and it ignores ticks and messages
// carrying another ID, so loops left over from an earlier start end
// instead of running alongside the new ones.
//
// A loader can move between models while running (the app shows one while
// it loads a screen's data, then hands it to that screen), and keeps its
// frame and message when it does.
type loader struct {
	spinner     spinner.Model
	picker      *loadingMessagePicker
	message     string
	ticking     bool
	rotating    bool
	messageText lipgloss.Style
}

// newLoader returns a stopped loader.
func newLoader() loader {
	return loader{spinner: spinner.New(spinner.WithSpinner(spinner.Dot))}
}

// loaderMessageMsg brings the loader's next message.
type loaderMessageMsg struct {
	id      int
	message string
}

// Start starts the loader afresh with messages, dropping any loops from
// before.
func (l *loader) Start(messages []string) tea.Cmd {
	style := l.spinner.Style
	l.spinner = spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(style))
	l.picker = newLoadingMessagePicker(messages)
	l.message = l.picker.next()
	l.ticking = true
	l.rotating = true
	return tea.Batch(l.spinner.Tick, l.nextMessage())
}

// Continue keeps a running loader going, such as one just handed over, and
// starts a stopped one with messages.
func (l *loader) Continue(messages []string) tea.Cmd {
	if l.ticking {
		return nil
	}
	return l.Start(messages)
}

// StopMessages keeps the spinner going but stops changing the message.
func (l *loader) StopMessages() {
	l.rotating = false
}

// Stop stops the spinner and the messages.
func (l *loader) Stop() {
	l.ticking = false
	l.rotating = false
}

// SetSpinner changes how the spinner looks without restarting it.
func (l *loader) SetSpinner(s spinner.Spinner) {
	l.spinner.Spinner = s
}

func (l *loader) SetStyles(s styles.Styles) {
	l.spinner.Style = s.Primary
	l.messageText = s.Accent
}

// Owns reports whether msg is one of this loader's ticks or messages.
func (l loader) Owns(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		return msg.ID == l.spinner.ID()
	case loaderMessageMsg:
		return msg.id == l.spinner.ID()
	}
	return false
}

func (l loader) Update(msg tea.Msg) (loader, tea.Cmd) {
	if !l.Owns(msg) {
		return l, nil
	}

	switch msg := msg.(type) {
	case spinner.TickMsg:
		if !l.ticking {
			return l, nil
		}
		var cmd tea.Cmd
		l.spinner, cmd = l.spinner.Update(msg)
		return l, cmd

	case loaderMessageMsg:
		if !l.rotating {
			return l, nil
		}
		l.message = msg.message
		return l, l.nextMessage()
	}

	return l, nil
}

// View is the spinner followed by the message.
func (l loader) View() string {
	return l.spinner.View() + " " + l.messageText.Render(l.message)
}

// SpinnerView is the spinner's current frame on its own.
func (l loader) SpinnerView() string {
	return l.spinner.View()
}

func (l loader) Message() string {
	return l.message
}

func (l *loader) nextMessage() tea.Cmd {
	return dispatchLoadingMessage(loaderMessageMsg{id: l.spinner.ID(), message: l.picker.next()})
}

// TakeOver continues from, a loader that is still running, in place of l.
// A stopped one is left alone, so l keeps its own styles and starts afresh.
func (l *loader) TakeOver(from loader) {
	if from.ticking {
		*l = from
	}
}
