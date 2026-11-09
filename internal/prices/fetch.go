package prices

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// SourceURL is models.dev's catalogue of models and their prices (MIT
// licensed, https://github.com/anomalyco/models.dev).
const SourceURL = "https://models.dev/api.json"

// maxDownload caps the download; the whole catalogue was about 6 MB in
// November 2026.
const maxDownload = 64 << 20

// Fetch downloads the models.dev catalogue from url and keeps the prices of
// Bark's providers' models, dated today. It fails if the download fails or
// lists no prices for them.
func Fetch(ctx context.Context, client *http.Client, url string, today time.Time) (File, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return File{}, err
	}
	req.Header.Set("User-Agent", "bark")
	resp, err := client.Do(req)
	if err != nil {
		return File{}, fmt.Errorf("downloading prices: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return File{}, fmt.Errorf("downloading prices from %s: %s", url, resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		return File{}, fmt.Errorf("downloading prices: %w", err)
	}
	if len(data) > maxDownload {
		return File{}, fmt.Errorf("downloading prices from %s: over %d MB", url, maxDownload>>20)
	}
	return Parse(data, url, today)
}

// Parse keeps the prices of Bark's providers' models from a models.dev
// catalogue. Models without input and output prices are left out. It fails
// if no model is left.
func Parse(data []byte, source string, today time.Time) (File, error) {
	var catalogue map[string]json.RawMessage
	if err := json.Unmarshal(data, &catalogue); err != nil {
		return File{}, fmt.Errorf("reading prices from %s: %w", source, err)
	}

	f := File{Source: source, Date: today.Format(time.DateOnly), Providers: map[string]map[string]Price{}}
	for _, id := range providerIDs {
		raw, ok := catalogue[id]
		if !ok {
			continue
		}
		var p mdProvider
		if err := json.Unmarshal(raw, &p); err != nil {
			return File{}, fmt.Errorf("reading %s prices from %s: %w", id, source, err)
		}
		models := map[string]Price{}
		for name, m := range p.Models {
			if price, ok := m.Cost.price(); ok {
				models[name] = price
			}
		}
		if len(models) > 0 {
			f.Providers[id] = models
		}
	}
	if f.Models() == 0 {
		return File{}, fmt.Errorf("no prices for Bark's providers in %s", source)
	}
	return f, nil
}

// Models counts the models f has prices for.
func (f File) Models() int {
	n := 0
	for _, models := range f.Providers {
		n += len(models)
	}
	return n
}

// Save writes f to path, creating its directory. It writes a temporary file
// first, so a failed save leaves an existing file as it was.
func Save(path string, f File) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("saving prices: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".prices-*.json")
	if err != nil {
		return fmt.Errorf("saving prices: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("saving prices: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("saving prices: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("saving prices: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("saving prices: %w", err)
	}
	return nil
}

// mdProvider is the part of a models.dev provider Bark reads.
type mdProvider struct {
	Models map[string]struct {
		Cost *mdCost `json:"cost"`
	} `json:"models"`
}

// mdRates are models.dev prices in USD per million tokens.
type mdRates struct {
	Input      *float64 `json:"input"`
	Output     *float64 `json:"output"`
	Reasoning  *float64 `json:"reasoning"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
}

type mdCost struct {
	mdRates
	Tiers []struct {
		mdRates
		Tier struct {
			Type string `json:"type"`
			Size int64  `json:"size"`
		} `json:"tier"`
	} `json:"tiers"`
	// ContextOver200k is the older form of a single 200k-token tier.
	ContextOver200k *mdRates `json:"context_over_200k"`
}

// price converts a models.dev cost; ok is false without input and output
// prices.
func (c *mdCost) price() (Price, bool) {
	if c == nil || c.Input == nil || c.Output == nil {
		return Price{}, false
	}
	p := Price{Rates: c.rates()}
	for _, t := range c.Tiers {
		if t.Tier.Type == "context" && t.Tier.Size > 0 {
			p.Tiers = append(p.Tiers, Tier{Over: t.Tier.Size, Rates: t.rates()})
		}
	}
	if len(p.Tiers) == 0 && c.ContextOver200k != nil {
		p.Tiers = []Tier{{Over: 200_000, Rates: c.ContextOver200k.rates()}}
	}
	slices.SortFunc(p.Tiers, func(a, b Tier) int { return int(a.Over - b.Over) })
	return p, true
}

func (r mdRates) rates() Rates {
	return Rates{
		Input:      value(r.Input),
		Output:     value(r.Output),
		Reasoning:  value(r.Reasoning),
		CacheRead:  value(r.CacheRead),
		CacheWrite: value(r.CacheWrite),
	}
}

func value(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
