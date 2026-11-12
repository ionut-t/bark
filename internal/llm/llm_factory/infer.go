package llm_factory

import (
	"regexp"
	"slices"
	"strings"
)

// oSeries matches OpenAI's reasoning models: o1, o3-mini, o4-mini, ...
var oSeries = regexp.MustCompile(`^o\d`)

// providersForModel returns the providers that serve model, in the order
// Bark prefers them, or nil when the name doesn't say. It only claims
// families whose names are unambiguous: Ollama can serve almost any name,
// so it is never inferred, and a name with an Ollama tag (gpt-oss:20b) is
// left alone.
func providersForModel(model string) []string {
	m := strings.ToLower(strings.TrimSpace(model))
	m = strings.TrimPrefix(m, "models/")

	switch {
	case strings.HasPrefix(m, "ft:"):
		// OpenAI fine-tunes: ft:gpt-4o-mini:org::id
		return []string{"openai"}
	case strings.Contains(m, ":"):
		return nil
	case strings.HasPrefix(m, "claude-"):
		return []string{"anthropic"}
	case strings.HasPrefix(m, "gemini-"):
		return []string{"gemini", "vertexai"}
	case strings.HasPrefix(m, "gpt-oss"):
		// Open weights; usually run locally.
		return nil
	case strings.HasPrefix(m, "gpt-"),
		strings.HasPrefix(m, "chatgpt-"),
		strings.HasPrefix(m, "codex-"),
		oSeries.MatchString(m):
		return []string{"openai"}
	}

	return nil
}

// resolveProvider picks the provider for model. A pinned provider (see
// config.Config.LLMProviderChoice) always wins. Otherwise a model whose name
// says which providers serve it decides: the configured provider if it is
// one of them, else the first with credentials, else the first (so the
// credentials error names the key to set). A name that says nothing falls
// back to the configured provider, then to detection by credentials.
func resolveProvider(provider string, pinned bool, model string, creds *providerCredentials) (string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if pinned && provider != "" {
		return provider, nil
	}

	candidates := providersForModel(model)
	if len(candidates) == 0 {
		if provider != "" {
			return provider, nil
		}
		return creds.detectProvider()
	}

	if slices.Contains(candidates, provider) {
		return provider, nil
	}
	for _, c := range candidates {
		if creds.has(c) {
			return c, nil
		}
	}
	return candidates[0], nil
}

// has reports whether the credentials for provider are set.
func (c *providerCredentials) has(provider string) bool {
	switch provider {
	case "gemini":
		return c.hasGemini
	case "vertexai":
		return c.hasVertexAI
	case "openai":
		return c.hasOpenAI
	case "anthropic":
		return c.hasAnthropic
	case "ollama":
		return c.hasOllama
	}
	return false
}
