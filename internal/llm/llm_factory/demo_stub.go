//go:build !demo

package llm_factory

import "github.com/ionut-t/bark/v2/internal/llm"

func demoOverride() (llm.LLM, string, bool) { return nil, "", false }
