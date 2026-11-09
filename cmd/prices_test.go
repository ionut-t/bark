package cmd

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/prices"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadPricesSavesBarkProviders(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"google": {"models": {"gemini-3.6-flash": {"cost": {"input": 0.75, "output": 3.75, "cache_read": 0.075}}}},
			"anthropic": {"models": {"claude-sonnet-5-5": {"cost": {"input": 2, "output": 10}}}},
			"openrouter": {"models": {"x": {"cost": {"input": 1, "output": 1}}}}
		}`))
	}))
	defer srv.Close()

	output := filepath.Join(t.TempDir(), ".bark", "prices.json")
	var out bytes.Buffer
	today := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	require.NoError(t, downloadPrices(context.Background(), srv.Client(), srv.URL, output, today, &out))

	assert.Equal(t, "Saved prices for 2 models to "+output+"\n", out.String())
	f, _, err := prices.Load(output)
	require.NoError(t, err)
	assert.Equal(t, "2026-11-09", f.Date)
	assert.Equal(t, srv.URL, f.Source)
	assert.InDelta(t, 3.75, f.Providers["google"]["gemini-3.6-flash"].Output, 0)
	assert.NotContains(t, f.Providers, "openrouter")
}

func TestDownloadPricesFailureKeepsTheExistingFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{"server error", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "down", http.StatusInternalServerError)
		}, "500"},
		{"no prices for Bark's providers", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{}`))
		}, "no prices for Bark's providers"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			output := filepath.Join(t.TempDir(), "prices.json")
			require.NoError(t, os.WriteFile(output, []byte("old"), 0o644))

			var out bytes.Buffer
			err := downloadPrices(context.Background(), srv.Client(), srv.URL, output, time.Now(), &out)
			require.ErrorContains(t, err, tt.want)
			assert.Empty(t, out.String())

			content, err := os.ReadFile(output)
			require.NoError(t, err)
			assert.Equal(t, "old", string(content))
		})
	}
}

func TestPricesRepoAndOutputAreExclusive(t *testing.T) {
	t.Parallel()

	_, err := runRootCmd(t, func(string) (config.Config, error) { return config.New("", config.Data{}), nil },
		"prices", "--repo", "--output", filepath.Join(t.TempDir(), "p.json"))
	require.ErrorContains(t, err, "none of the others can be")
}

func TestTopLevelWarning(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sub := filepath.Join(root, "internal")
	require.NoError(t, os.Mkdir(sub, 0o755))

	assert.Empty(t, topLevelWarning(root, root, nil))
	assert.Empty(t, topLevelWarning(root+string(filepath.Separator), root, nil))

	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(root, link))
	assert.Empty(t, topLevelWarning(link, root, nil), "a symlink to the top level is the top level")

	assert.Equal(t, sub+" is not the repository's top level ("+root+"); Bark reads .bark/prices.json from the directory it runs in",
		topLevelWarning(sub, root, nil))
	assert.Equal(t, "not in a git repository; saving to .bark/prices.json in the current directory",
		topLevelWarning(root, "", errors.New("not a repo")))
}
