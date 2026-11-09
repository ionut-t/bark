package prices

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var today = time.Date(2026, 11, 9, 15, 0, 0, 0, time.UTC)

func sample(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "models.dev.json"))
	require.NoError(t, err)
	return data
}

func TestParseKeepsBarkProvidersPrices(t *testing.T) {
	t.Parallel()

	f, err := Parse(sample(t), SourceURL, today)
	require.NoError(t, err)

	assert.Equal(t, SourceURL, f.Source)
	assert.Equal(t, "2026-11-09", f.Date)
	assert.ElementsMatch(t, []string{"google", "google-vertex", "anthropic", "openai"}, keys(f.Providers))
	assert.Equal(t, 7, f.Models())

	google := f.Providers["google"]
	assert.NotContains(t, google, "gemini-embedding", "a model without prices is left out")
	assert.Equal(t, Price{Input: 0.75, Output: 3.75, CacheRead: 0.075}, google["gemini-3.6-flash"])
	assert.Equal(t, Price{
		Input: 2, Output: 12, CacheRead: 0.2,
		Tiers: []Tier{{Over: 200_000, Input: 4, Output: 18, CacheRead: 0.4}},
	}, google["gemini-3.1-pro-preview"])
	assert.Equal(t, []Tier{{Over: 200_000, Input: 2.5, Output: 15}}, google["gemini-old-pro"].Tiers,
		"context_over_200k is used when there are no tiers")
	assert.Equal(t, []Tier{{Over: 272_000, Input: 10, Output: 45, CacheRead: 1}}, f.Providers["openai"]["gpt-5.5"].Tiers,
		"tiers win over context_over_200k")
}

func TestParseFailsWithoutBarkProviders(t *testing.T) {
	t.Parallel()

	_, err := Parse([]byte(`{"openrouter":{"models":{"m":{"cost":{"input":1,"output":2}}}}}`), "x", today)
	require.ErrorContains(t, err, "no prices for Bark's providers in x")

	_, err = Parse([]byte(`<html>`), "x", today)
	require.Error(t, err)
}

func TestFetch(t *testing.T) {
	t.Parallel()

	data := sample(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api.json" {
			http.Error(w, "gone", http.StatusNotFound)
			return
		}
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	f, err := Fetch(context.Background(), srv.Client(), srv.URL+"/api.json", today)
	require.NoError(t, err)
	assert.Equal(t, 7, f.Models())

	_, err = Fetch(context.Background(), srv.Client(), srv.URL+"/missing.json", today)
	require.ErrorContains(t, err, "404")
}

func TestSaveAndLoad(t *testing.T) {
	t.Parallel()

	f, err := Parse(sample(t), SourceURL, today)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "nested", FileName)
	require.NoError(t, Save(path, f))

	got, gotPath, err := Load(filepath.Join(t.TempDir(), "none.json"), path)
	require.NoError(t, err)
	assert.Equal(t, path, gotPath)
	assert.Equal(t, f, *got)

	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temporary file is left behind")
}

func TestLoadWithoutFiles(t *testing.T) {
	t.Parallel()

	f, path, err := Load(filepath.Join(t.TempDir(), "none.json"))
	require.NoError(t, err)
	assert.Nil(t, f)
	assert.Empty(t, path)
}

func TestCost(t *testing.T) {
	t.Parallel()

	flash := Price{Input: 0.75, Output: 3.75, CacheRead: 0.075}
	pro := Price{
		Input: 2, Output: 12, CacheRead: 0.2,
		Tiers: []Tier{{Over: 200_000, Input: 4, Output: 18, CacheRead: 0.4}},
	}
	haiku := Price{
		Input: 0.1, Output: 0.5, CacheRead: 0.01, CacheWrite: 0.125,
		Tiers: []Tier{{Over: 100_000, Input: 0.5, Output: 2.5, CacheRead: 0.05, CacheWrite: 0.625}},
	}

	tests := []struct {
		name  string
		price Price
		usage llm.Usage
		want  float64
	}{
		{
			name:  "thinking outside the output is billed at the output price",
			price: flash,
			usage: llm.Usage{InputTokens: 10_000, OutputTokens: 1_000, TotalTokens: 13_000},
			want:  (10_000*0.75 + 1_000*3.75 + 2_000*3.75) / 1e6,
		},
		{
			name:  "cached input at the cache price",
			price: flash,
			usage: llm.Usage{InputTokens: 10_000, CachedInputTokens: 4_000, OutputTokens: 100, TotalTokens: 10_100},
			want:  (6_000*0.75 + 4_000*0.075 + 100*3.75) / 1e6,
		},
		{
			name:  "a prompt at the tier's size keeps the base prices",
			price: pro,
			usage: llm.Usage{InputTokens: 200_000, OutputTokens: 1_000, TotalTokens: 201_000},
			want:  (200_000*2 + 1_000*12) / 1e6,
		},
		{
			name:  "a prompt over the tier's size pays the tier's prices for all of it",
			price: pro,
			usage: llm.Usage{InputTokens: 200_001, OutputTokens: 1_000, TotalTokens: 201_001},
			want:  (200_001*4 + 1_000*18) / 1e6,
		},
		{
			name:  "cached tokens count towards the tier",
			price: haiku,
			usage: llm.Usage{InputTokens: 120_000, CachedInputTokens: 90_000, CacheWriteTokens: 10_000, OutputTokens: 500, TotalTokens: 120_500},
			want:  (20_000*0.5 + 90_000*0.05 + 10_000*0.625 + 500*2.5) / 1e6,
		},
		{
			name:  "without cache prices cached tokens pay the input price",
			price: Price{Rates: Rates{Input: 1, Output: 2}},
			usage: llm.Usage{InputTokens: 1_000, CachedInputTokens: 500, CacheWriteTokens: 200, OutputTokens: 10, TotalTokens: 1_010},
			want:  (1_000*1 + 10*2) / 1e6,
		},
		{
			name:  "a reasoning price is used for thinking",
			price: Price{Rates: Rates{Input: 1, Output: 2, Reasoning: 3}},
			usage: llm.Usage{InputTokens: 100, OutputTokens: 10, TotalTokens: 1_110},
			want:  (100*1 + 10*2 + 1_000*3) / 1e6,
		},
		{
			name:  "a tier without a cache price keeps the base one",
			price: Price{Rates: Rates{Input: 1, Output: 2, CacheRead: 0.1}, Tiers: []Tier{{Over: 10, Input: 3, Output: 4}}},
			usage: llm.Usage{InputTokens: 100, CachedInputTokens: 50, OutputTokens: 10, TotalTokens: 110},
			want:  (50*3 + 50*0.1 + 10*4) / 1e6,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.InDelta(t, tt.want, tt.price.Cost(tt.usage), 1e-12)
		})
	}
}

func TestCostPicksTheHighestTierPassed(t *testing.T) {
	t.Parallel()

	// Unordered on purpose: the highest size the prompt is over wins.
	p := Price{Input: 1, Output: 1, Tiers: []Tier{
		{Over: 128_000, Input: 3, Output: 3},
		{Over: 32_000, Input: 2, Output: 2},
	}}
	assert.InDelta(t, 10_000*1/1e6, p.Cost(llm.Usage{InputTokens: 10_000, TotalTokens: 10_000}), 1e-12)
	assert.InDelta(t, 50_000*2/1e6, p.Cost(llm.Usage{InputTokens: 50_000, TotalTokens: 50_000}), 1e-12)
	assert.InDelta(t, 150_000*3/1e6, p.Cost(llm.Usage{InputTokens: 150_000, TotalTokens: 150_000}), 1e-12)
}

func TestFormatUSD(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "$0.0000", FormatUSD(0))
	assert.Equal(t, "$0.0123", FormatUSD(0.01234))
	assert.Equal(t, "$0.9999", FormatUSD(0.99991))
	assert.Equal(t, "$1.50", FormatUSD(1.5))
}

// For reads relative paths, so these tests change the working directory
// and can't run in parallel.
func TestForPrefersTheRepoFile(t *testing.T) {
	repo, storage := t.TempDir(), t.TempDir()
	t.Chdir(repo)

	writePrices(t, filepath.Join(storage, FileName), "2026-10-01", 1)
	writePrices(t, RepoPath, "2026-11-09", 2)

	p, err := For(storage, "gemini", "gemini-3.6-flash")
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "2026-11-09", p.Date)
	assert.Equal(t, RepoPath, p.Path)
	assert.InDelta(t, 2.0, p.Price.Input, 0)

	require.NoError(t, os.Remove(RepoPath))
	p, err = For(storage, "gemini", "models/gemini-3.6-flash")
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "2026-10-01", p.Date)
	assert.Equal(t, filepath.Join(storage, FileName), p.Path)
}

func TestForProvidersAndMissingPrices(t *testing.T) {
	storage := t.TempDir()
	t.Chdir(t.TempDir())

	p, err := For(storage, "gemini", "gemini-3.6-flash")
	require.NoError(t, err, "no prices file is not an error")
	assert.Nil(t, p)

	writePrices(t, filepath.Join(storage, FileName), "2026-11-09", 1)

	p, err = For(storage, "ollama", "gemma3:12b")
	require.NoError(t, err, "Ollama has no prices")
	assert.Nil(t, p)

	p, err = For(storage, "vertexai", "gemini-3.6-flash")
	require.NoError(t, err)
	assert.NotNil(t, p, "Vertex AI reads google-vertex")

	_, err = For(storage, "gemini", "gemini-9")
	require.ErrorIs(t, err, ErrNotListed)
	assert.ErrorContains(t, err, "gemini model gemini-9")

	require.NoError(t, os.WriteFile(filepath.Join(storage, FileName), []byte("{"), 0o644))
	_, err = For(storage, "gemini", "gemini-3.6-flash")
	require.ErrorContains(t, err, "reading prices from")
}

func writePrices(t *testing.T, path, date string, input float64) {
	t.Helper()
	price := Price{Input: input, Output: 1}
	require.NoError(t, Save(path, File{Source: SourceURL, Date: date, Providers: map[string]map[string]Price{
		"google":        {"gemini-3.6-flash": price},
		"google-vertex": {"gemini-3.6-flash": price},
	}}))
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
