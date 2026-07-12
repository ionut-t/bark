package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/stretchr/testify/require"
)

// Deltas arriving within the coalesce window must be batched into one message.
func TestWatchStreamCoalescesDeltas(t *testing.T) {
	respChan := make(chan llm.Response)
	errChan := make(chan error, 1)

	go func() {
		for _, s := range []string{"a", "b", "c", "d"} {
			respChan <- llm.Response{Content: s, Time: time.Now()}
			time.Sleep(5 * time.Millisecond)
		}
	}()

	msg := watchStreamCmd(respChan, errChan)()
	require.IsType(t, streamChunkMsg{}, msg)
	require.Equal(t, "abcd", msg.(streamChunkMsg).content)
}

// Providers close errChan before respChan (defers run LIFO); the watch must
// keep draining respChan instead of dying on the closed error channel.
func TestWatchStreamSurvivesErrChanClosingFirst(t *testing.T) {
	respChan := make(chan llm.Response, 1)
	errChan := make(chan error)
	close(errChan)

	respChan <- llm.Response{Content: "tail", Time: time.Now()}

	msg := watchStreamCmd(respChan, errChan)()
	require.IsType(t, streamChunkMsg{}, msg)
	require.Equal(t, "tail", msg.(streamChunkMsg).content)

	close(respChan)
	require.Equal(t, streamCompleteMsg{}, watchStreamCmd(respChan, errChan)())
}

// Content buffered when respChan closes mid-window must be flushed, with
// completion reported by the following watch.
func TestWatchStreamFlushesBufferOnClose(t *testing.T) {
	respChan := make(chan llm.Response, 2)
	errChan := make(chan error, 1)

	respChan <- llm.Response{Content: "last ", Time: time.Now()}
	respChan <- llm.Response{Content: "words", Time: time.Now()}
	close(respChan)
	close(errChan)

	msg := watchStreamCmd(respChan, errChan)()
	require.IsType(t, streamChunkMsg{}, msg)
	require.Equal(t, "last words", msg.(streamChunkMsg).content)

	require.Equal(t, streamCompleteMsg{}, watchStreamCmd(respChan, errChan)())
}

// An error buffered just before the provider closes both channels must be
// surfaced, not randomly swallowed as a normal completion.
func TestWatchStreamPrefersPendingErrorOverCompletion(t *testing.T) {
	boom := errors.New("boom")

	for range 20 {
		respChan := make(chan llm.Response)
		errChan := make(chan error, 1)
		errChan <- boom
		close(errChan)
		close(respChan)

		msg := watchStreamCmd(respChan, errChan)()
		require.IsType(t, streamErrorMsg{}, msg)
		require.ErrorIs(t, msg.(streamErrorMsg).error, boom)
	}
}

// A buffered context cancellation still reads as a normal completion.
func TestWatchStreamTreatsCancellationAsCompletion(t *testing.T) {
	respChan := make(chan llm.Response)
	errChan := make(chan error, 1)
	errChan <- context.Canceled
	close(errChan)
	close(respChan)

	require.Equal(t, streamCompleteMsg{}, watchStreamCmd(respChan, errChan)())
}

// A final response bearing usage metadata must propagate to the completion message.
func TestWatchStreamPropagatesUsage(t *testing.T) {
	respChan := make(chan llm.Response, 1)
	errChan := make(chan error)

	usage := llm.Usage{InputTokens: 10, OutputTokens: 20, TotalTokens: 30}
	respChan <- llm.Response{Usage: &usage, Time: time.Now()}
	close(respChan)
	close(errChan)

	msg := watchStreamCmd(respChan, errChan)()
	require.IsType(t, streamCompleteMsg{}, msg)
	require.True(t, msg.(streamCompleteMsg).hasUsage)
	require.Equal(t, usage, msg.(streamCompleteMsg).usage)
}

// Usage arriving alongside content mid-stream must ride out on the coalesced
// chunk message, not be lost when the deadline flushes the buffer.
func TestWatchStreamPropagatesUsageOnChunk(t *testing.T) {
	respChan := make(chan llm.Response, 1)
	errChan := make(chan error, 1)

	usage := llm.Usage{InputTokens: 10, OutputTokens: 20, TotalTokens: 30}
	respChan <- llm.Response{Content: "tail", Usage: &usage, Time: time.Now()}

	msg := watchStreamCmd(respChan, errChan)()
	require.IsType(t, streamChunkMsg{}, msg)
	chunk := msg.(streamChunkMsg)
	require.Equal(t, "tail", chunk.content)
	require.True(t, chunk.hasUsage)
	require.Equal(t, usage, chunk.usage)
}

// A mid-stream error must surface as streamErrorMsg.
func TestWatchStreamReportsError(t *testing.T) {
	respChan := make(chan llm.Response)
	errChan := make(chan error, 1)

	boom := errors.New("boom")
	errChan <- boom

	msg := watchStreamCmd(respChan, errChan)()
	require.IsType(t, streamErrorMsg{}, msg)
	require.ErrorIs(t, msg.(streamErrorMsg).error, boom)
}
