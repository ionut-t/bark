package tui

import (
	"testing"

	"github.com/ionut-t/bark/v2/internal/config"
	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/ionut-t/bark/v2/internal/prices"
	"github.com/ionut-t/coffee/styles"
	"github.com/stretchr/testify/assert"
)

// A dollar per million input tokens and two per million output tokens.
var testPricing = &prices.Pricing{Price: prices.Price{Input: 1, Output: 2}, Date: "2026-11-09"}

func TestRecordCheckUsageAddsTheCheckToTheReview(t *testing.T) {
	t.Parallel()

	m := &Model{pricing: testPricing, config: config.New("", config.Data{})}
	m.review.run, m.review.checking = 2, true
	m.recordUsage(llm.Usage{InputTokens: 10_000, OutputTokens: 1_000, TotalTokens: 11_000}, 0)

	m.recordCheckUsage(checkDoneMsg{run: 1, usage: &llm.Usage{InputTokens: 1}})
	assert.Nil(t, m.lastUsage.check, "a check for a restarted review is ignored")

	m.recordCheckUsage(checkDoneMsg{run: 2, usage: &llm.Usage{InputTokens: 30_000, OutputTokens: 500, TotalTokens: 30_500}})
	if assert.NotNil(t, m.lastUsage.check) {
		assert.Equal(t, int64(30_000), m.lastUsage.check.InputTokens)
	}
	assert.Equal(t, "$0.0430: review $0.0120, check $0.0310 (prices from 2026-11-09)", costSummary(m.lastUsage, testPricing))

	m.recordUsage(llm.Usage{InputTokens: 1_000, TotalTokens: 1_000}, 0)
	assert.Nil(t, m.lastUsage.check, "the next review starts without a check")
	assert.Equal(t, "$0.0010 (prices from 2026-11-09)", costSummary(m.lastUsage, testPricing))
}

func TestRenderUsageStatsShowsTheCheck(t *testing.T) {
	t.Parallel()

	s := styles.New(true)
	stats := &usageStats{
		usage:   llm.Usage{InputTokens: 10_000, OutputTokens: 1_000, TotalTokens: 11_000},
		check:   &llm.Usage{InputTokens: 30_000, OutputTokens: 500, TotalTokens: 30_500},
		pricing: testPricing,
	}
	got := renderUsageStats(stats, s)
	assert.Contains(t, got, "30,000 input, 500 output tokens")
	assert.Contains(t, got, "review $0.0120, check $0.0310")

	stats.pricing = nil
	assert.NotContains(t, renderUsageStats(stats, s), "Cost:")
}
