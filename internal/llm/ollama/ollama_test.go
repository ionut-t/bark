package ollama

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ionut-t/bark/v2/internal/llm/llmtest"
)

type failingCloseBody struct{ io.Reader }

func (failingCloseBody) Close() error { return llmtest.ErrClose }

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestStream_StreamErrorAndCloseErrorDoNotHang(t *testing.T) {
	o := New("llama-test")
	o.host = "http://llm.test"
	o.client = &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		body := `{"error":"boom"}` + "\n"
		if r.URL.Path == "/api/show" {
			body = `{"model_info":{}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: failingCloseBody{strings.NewReader(body)}, Request: r}, nil
	})}

	out, errc := o.Stream(t.Context(), "", "prompt")
	llmtest.AssertStreamCloses(t, out, errc)
}
