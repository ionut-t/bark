package llm_factory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/ionut-t/bark/v2/internal/llm/anthropic"
	"github.com/ionut-t/bark/v2/internal/llm/gemini"
	"github.com/ionut-t/bark/v2/internal/llm/ollama"
	"github.com/ionut-t/bark/v2/internal/llm/openai"
	"github.com/ionut-t/bark/v2/internal/llm/vertexai"
)

var (
	ErrNoProviderConfigured = errors.New("no LLM provider configured")
	ErrInvalidProvider      = errors.New("unsupported LLM provider")
	ErrMissingCredentials   = errors.New("missing provider credentials")
)

// providerCredentials holds the environment variable values for different providers
type providerCredentials struct {
	geminiAPIKey      string
	vertexAIProjectID string
	vertexAILocation  string
	openAIAPIKey      string
	ollamaHost        string
	anthropicAPIKey   string
	hasGemini         bool
	hasVertexAI       bool
	hasOpenAI         bool
	hasOllama         bool
	hasAnthropic      bool
}

// loadCredentials reads and validates environment variables
func loadCredentials() *providerCredentials {
	creds := &providerCredentials{
		geminiAPIKey:      os.Getenv("GEMINI_API_KEY"),
		vertexAIProjectID: os.Getenv("VERTEXAI_PROJECT_ID"),
		vertexAILocation:  os.Getenv("VERTEXAI_LOCATION"),
		openAIAPIKey:      os.Getenv("OPENAI_API_KEY"),
		ollamaHost:        os.Getenv("OLLAMA_HOST"),
		anthropicAPIKey:   os.Getenv("ANTHROPIC_API_KEY"),
	}

	creds.hasGemini = creds.geminiAPIKey != ""
	creds.hasVertexAI = creds.vertexAIProjectID != "" && creds.vertexAILocation != ""
	creds.hasOpenAI = creds.openAIAPIKey != ""
	creds.hasOllama = creds.ollamaHost != ""
	creds.hasAnthropic = creds.anthropicAPIKey != ""

	return creds
}

// detectProvider automatically detects which provider to use based on available credentials
func (c *providerCredentials) detectProvider() (string, error) {
	if c.hasGemini {
		return "gemini", nil
	}
	if c.hasVertexAI {
		return "vertexai", nil
	}
	if c.hasOpenAI {
		return "openai", nil
	}
	if c.hasAnthropic {
		return "anthropic", nil
	}
	if c.hasOllama {
		return "ollama", nil
	}
	return "", fmt.Errorf("%w: set GEMINI_API_KEY, OPENAI_API_KEY, ANTHROPIC_API_KEY, OLLAMA_HOST, or both VERTEXAI_PROJECT_ID and VERTEXAI_LOCATION", ErrNoProviderConfigured)
}

// validateProvider checks if credentials exist for the specified provider
func (c *providerCredentials) validateProvider(provider string) error {
	switch provider {
	case "gemini":
		if !c.hasGemini {
			return fmt.Errorf("%w for Gemini: GEMINI_API_KEY not set", ErrMissingCredentials)
		}
	case "vertexai":
		if !c.hasVertexAI {
			missing := []string{}
			if c.vertexAIProjectID == "" {
				missing = append(missing, "VERTEXAI_PROJECT_ID")
			}
			if c.vertexAILocation == "" {
				missing = append(missing, "VERTEXAI_LOCATION")
			}
			return fmt.Errorf("%w for Vertex AI: %s not set", ErrMissingCredentials, strings.Join(missing, " and "))
		}
	case "openai":
		if !c.hasOpenAI {
			return fmt.Errorf("%w for OpenAI: OPENAI_API_KEY not set", ErrMissingCredentials)
		}
	case "anthropic":
		if !c.hasAnthropic {
			return fmt.Errorf("%w for Anthropic: ANTHROPIC_API_KEY not set", ErrMissingCredentials)
		}
	case "ollama":
		// Ollama runs locally; no credentials required
	default:
		return fmt.Errorf("%w: %s (supported: gemini, vertexai, openai, anthropic, ollama)", ErrInvalidProvider, provider)
	}

	return nil
}

func New(ctx context.Context, cfg config.Config) (llm.LLM, error) {
	creds := loadCredentials()

	provider, err := cfg.GetLLMProvider()
	if err != nil || provider == "" {
		// Config doesn't specify provider, try to auto-detect
		provider, err = creds.detectProvider()
		if err != nil {
			return nil, err
		}
	}

	provider = strings.ToLower(strings.TrimSpace(provider))

	if err := creds.validateProvider(provider); err != nil {
		return nil, err
	}

	model, err := cfg.GetLLMModel()
	if err != nil {
		return nil, err
	}

	switch provider {
	case "gemini":
		return gemini.New(ctx, model, creds.geminiAPIKey)
	case "vertexai":
		return vertexai.New(ctx, model, creds.vertexAIProjectID, creds.vertexAILocation)
	case "openai":
		return openai.New(model, creds.openAIAPIKey), nil
	case "anthropic":
		return anthropic.New(model, creds.anthropicAPIKey), nil
	case "ollama":
		return ollama.New(model), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidProvider, provider)
	}
}
