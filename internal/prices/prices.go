// Package prices works out what a model call cost from its token usage and
// a prices file that `bark prices` saves from models.dev. Bark never
// downloads prices on its own: without a prices file, or for a model the
// file doesn't list (and for Ollama, which is local), there is no cost.
package prices

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ionut-t/bark/v2/internal/llm"
)

// FileName is the prices file's name, in a repo's .bark directory or in
// Bark's storage directory.
const FileName = "prices.json"

// RepoPath is the repo's own prices file, read before the one in Bark's
// storage directory, so CI can use prices committed to the repo.
var RepoPath = filepath.Join(".bark", FileName)

// File is a saved prices file.
type File struct {
	// Source says where the prices came from.
	Source string `json:"source"`
	// Date is the day they were downloaded (YYYY-MM-DD).
	Date string `json:"date"`
	// Providers maps a models.dev provider id (see providerIDs) to its
	// models' prices by model id.
	Providers map[string]map[string]Price `json:"providers"`
}

// Price is one model's prices in USD per million tokens. Every price is
// set, and zero means free: prices models.dev leaves out are filled in when
// they are saved (see Parse).
type Price struct {
	Rates
	// Tiers hold higher prices for long prompts, ordered by Over.
	Tiers []Tier `json:"tiers,omitempty"`
}

// Rates are prices in USD per million tokens. Reasoning is the price of
// thinking that isn't counted in the output (Gemini's).
type Rates struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	Reasoning  float64 `json:"reasoning"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
}

// Tier is the prices for prompts over a number of tokens (cached ones
// included).
type Tier struct {
	Over int64 `json:"over"`
	Rates
}

// providerIDs maps Bark's provider names to models.dev's provider ids.
// Ollama runs locally and has no prices.
var providerIDs = map[string]string{
	"gemini":    "google",
	"vertexai":  "google-vertex",
	"anthropic": "anthropic",
	"openai":    "openai",
}

// Pricing is one model's prices and where they came from.
type Pricing struct {
	Price Price
	// Date is the day the prices were downloaded, and Path the file they
	// were read from.
	Date, Path string
}

// ErrNotListed is returned by For when the prices file has no prices for
// the model.
var ErrNotListed = errors.New("no price listed")

// For returns the prices for provider's model from the repo's prices file
// (RepoPath) or, without one, from storage's. It returns nil and no error
// when there is no prices file or the provider has no prices (Ollama). An
// unreadable file is an error, and so is a model the file doesn't list
// (wrapping ErrNotListed).
func For(storage, provider, model string) (*Pricing, error) {
	id, ok := providerIDs[provider]
	if !ok {
		return nil, nil
	}
	file, path, err := Load(RepoPath, filepath.Join(storage, FileName))
	if err != nil || file == nil {
		return nil, err
	}
	price, ok := file.Providers[id][strings.TrimPrefix(model, "models/")]
	if !ok {
		return nil, fmt.Errorf("%w for %s model %s in %s", ErrNotListed, provider, model, path)
	}
	return &Pricing{Price: price, Date: file.Date, Path: path}, nil
}

// Load reads the first of paths that exists and returns it with its path.
// It returns a nil File when none exists.
func Load(paths ...string) (*File, string, error) {
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, "", fmt.Errorf("reading prices: %w", err)
		}
		var f File
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, "", fmt.Errorf("reading prices from %s: %w", path, err)
		}
		return &f, path, nil
	}
	return nil, "", nil
}

// Cost is what a call with usage u cost in USD. The rates are those of the
// highest tier the prompt is over. Cached input is billed at the cache
// prices, and thinking that isn't counted in the output (Gemini) at the
// reasoning price.
func (p Price) Cost(u llm.Usage) float64 {
	r := p.ratesFor(u.InputTokens)
	uncached := max(0, u.InputTokens-u.CachedInputTokens-u.CacheWriteTokens)
	usd := float64(uncached)*r.Input +
		float64(u.CachedInputTokens)*r.CacheRead +
		float64(u.CacheWriteTokens)*r.CacheWrite +
		float64(u.OutputTokens)*r.Output +
		float64(u.ThinkingTokens())*r.Reasoning
	return usd / 1e6
}

// ratesFor returns the rates for a prompt of input tokens.
func (p Price) ratesFor(input int64) Rates {
	var tier *Tier
	for i := range p.Tiers {
		if input > p.Tiers[i].Over && (tier == nil || p.Tiers[i].Over > tier.Over) {
			tier = &p.Tiers[i]
		}
	}
	if tier == nil {
		return p.Rates
	}
	return tier.Rates
}

// Cost is what a call with usage u cost in USD (see Price.Cost).
func (p *Pricing) Cost(u llm.Usage) float64 {
	return p.Price.Cost(u)
}

// FormatUSD formats a cost: four decimals below a dollar, so small calls
// don't show as $0.00, two from a dollar up.
func FormatUSD(usd float64) string {
	if usd < 1 {
		return fmt.Sprintf("$%.4f", usd)
	}
	return fmt.Sprintf("$%.2f", usd)
}
