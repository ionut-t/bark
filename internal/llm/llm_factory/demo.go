//go:build demo

package llm_factory

import (
	"os"

	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/ionut-t/bark/v2/internal/llm/mock"
)

// demoOverride returns the offline mock LLM when BARK_DEMO_MOCK=1. It exists
// only in binaries built with the "demo" tag (see demo/record.sh).
func demoOverride() (llm.LLM, string, bool) {
	if os.Getenv("BARK_DEMO_MOCK") != "1" {
		return nil, "", false
	}
	return mock.New(), "mock", true
}
