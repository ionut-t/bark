package tui

import (
	"testing"

	"charm.land/bubbles/v2/spinner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testMessages = []string{"one", "two", "three"}

// rotation is the message l's rotation delivers after 5s.
func rotation(l loader) loaderMessageMsg {
	return loaderMessageMsg{id: l.spinner.ID(), message: l.picker.next()}
}

func TestLoader(t *testing.T) {
	t.Run("start runs the spinner and the messages", func(t *testing.T) {
		l := newLoader()
		require.NotNil(t, l.Start(testMessages))
		assert.Contains(t, testMessages, l.Message())
		assert.Contains(t, l.View(), l.Message())

		l, cmd := l.Update(l.spinner.Tick())
		assert.NotNil(t, cmd, "the spinner should schedule its next frame")

		before := l.Message()
		l, cmd = l.Update(rotation(l))
		assert.NotNil(t, cmd, "the messages should schedule the next one")
		assert.NotEqual(t, before, l.Message())
	})

	t.Run("start again drops the earlier loops", func(t *testing.T) {
		l := newLoader()
		_ = l.Start(testMessages)
		earlier := l
		_ = l.Start(testMessages)

		before := l.Message()
		l, cmd := l.Update(rotation(earlier))
		assert.Nil(t, cmd)
		assert.Equal(t, before, l.Message())
		_, cmd = l.Update(earlier.spinner.Tick())
		assert.Nil(t, cmd)
	})

	t.Run("continue keeps a running loader", func(t *testing.T) {
		l := newLoader()
		_ = l.Start(testMessages)
		id, msg := l.spinner.ID(), l.Message()

		assert.Nil(t, l.Continue([]string{"other"}), "no second set of loops")
		assert.Equal(t, id, l.spinner.ID())
		assert.Equal(t, msg, l.Message())
	})

	t.Run("continue starts a stopped loader", func(t *testing.T) {
		l := newLoader()
		_ = l.Start(testMessages)
		l.Stop()

		require.NotNil(t, l.Continue([]string{"other"}))
		assert.Equal(t, "other", l.Message())
	})

	t.Run("stop messages keeps the spinner", func(t *testing.T) {
		l := newLoader()
		_ = l.Start(testMessages)
		l.StopMessages()
		l.SetSpinner(spinner.Points)

		l, cmd := l.Update(rotation(l))
		assert.Nil(t, cmd)
		_, cmd = l.Update(l.spinner.Tick())
		assert.NotNil(t, cmd)
	})

	t.Run("stop ends both", func(t *testing.T) {
		l := newLoader()
		_ = l.Start(testMessages)
		l.Stop()

		l, cmd := l.Update(rotation(l))
		assert.Nil(t, cmd)
		_, cmd = l.Update(l.spinner.Tick())
		assert.Nil(t, cmd)
	})

	t.Run("ignores other loaders", func(t *testing.T) {
		a, b := newLoader(), newLoader()
		_ = a.Start(testMessages)
		_ = b.Start(testMessages)

		assert.False(t, a.Owns(b.spinner.Tick()))
		assert.False(t, a.Owns(rotation(b)))
		_, cmd := a.Update(b.spinner.Tick())
		assert.Nil(t, cmd)
	})

	t.Run("take over continues a running loader", func(t *testing.T) {
		from := newLoader()
		_ = from.Start(testMessages)
		l := newLoader()
		l.TakeOver(from)

		assert.Equal(t, from.spinner.ID(), l.spinner.ID())
		assert.Equal(t, from.Message(), l.Message())
		_, cmd := l.Update(from.spinner.Tick())
		assert.NotNil(t, cmd)
	})

	t.Run("take over ignores a stopped loader", func(t *testing.T) {
		l := newLoader()
		id := l.spinner.ID()
		l.TakeOver(loader{})
		assert.Equal(t, id, l.spinner.ID())
	})
}
