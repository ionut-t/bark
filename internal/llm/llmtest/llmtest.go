// Package llmtest holds helpers shared by the LLM provider tests.
package llmtest

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ErrClose is returned by the body of responses from StreamErrorClient.
var ErrClose = io.ErrClosedPipe

// StreamErrorEvent is an SSE error event that both the Anthropic and OpenAI
// SDK streams turn into a stream error.
const StreamErrorEvent = "event: error\n" +
	`data: {"type":"error","error":{"type":"overloaded_error","message":"boom"}}` + "\n\n"

type failingCloseBody struct{ io.Reader }

func (failingCloseBody) Close() error { return ErrClose }

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// StreamErrorClient returns an HTTP client whose every response is a 200 SSE
// stream carrying StreamErrorEvent, with a body whose Close fails.
func StreamErrorClient() *http.Client {
	return &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       failingCloseBody{strings.NewReader(StreamErrorEvent)},
			Request:    r,
		}, nil
	})}
}

// JSONClient returns an HTTP client whose every response is a 200 with body
// as its JSON payload.
func JSONClient(body string) *http.Client {
	return &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})}
}

// AssertStreamCloses drains out without reading errc, then checks that both
// channels are closed within a timeout and that errc holds exactly one error.
func AssertStreamCloses(t *testing.T, out <-chan llm.Response, errc <-chan error) {
	t.Helper()

	timeout := time.After(5 * time.Second)
	for open := true; open; {
		select {
		case _, open = <-out:
		case <-timeout:
			require.FailNow(t, "response channel was not closed; the stream goroutine is stuck")
		}
	}

	var errs []error
	for open := true; open; {
		select {
		case err, ok := <-errc:
			if ok {
				errs = append(errs, err)
			}
			open = ok
		case <-timeout:
			require.FailNow(t, "error channel was not closed")
		}
	}

	require.Len(t, errs, 1)
	assert.ErrorContains(t, errs[0], "boom", "the stream error should win over the Close error")
}
