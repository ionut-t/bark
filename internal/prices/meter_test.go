package prices

import (
	"context"
	"errors"
	"testing"

	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeLLM answers every call with usage, streamed in two chunks with the
// usage on the last one, as the providers do.
type fakeLLM struct {
	usage *llm.Usage
	err   error
}

func (f fakeLLM) Generate(context.Context, string, string) (llm.Response, error) {
	return llm.Response{Content: "ok", Usage: f.usage}, f.err
}

func (f fakeLLM) Stream(context.Context, string, string) (<-chan llm.Response, <-chan error) {
	out := make(chan llm.Response, 2)
	errc := make(chan error, 1)
	out <- llm.Response{Content: "o"}
	out <- llm.Response{Content: "k", Usage: f.usage}
	close(out)
	if f.err != nil {
		errc <- f.err
	}
	close(errc)
	return out, errc
}

// A dollar per million input tokens and two per million output tokens.
var testPricing = &Pricing{Price: Price{Input: 1, Output: 2}, Date: "2026-11-09"}

func TestMeterCountsGenerateAndStream(t *testing.T) {
	t.Parallel()

	usage := &llm.Usage{InputTokens: 1_000, OutputTokens: 500, TotalTokens: 1_500}
	m := NewMeter(fakeLLM{usage: usage}, testPricing)

	resp, err := m.Generate(context.Background(), "s", "p")
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Content)
	m.Mark("review")

	out, errc := m.Stream(context.Background(), "s", "p")
	var content string
	for r := range out {
		content += r.Content
	}
	require.NoError(t, <-errc)
	assert.Equal(t, "ok", content)
	// The stream's cost is counted before its channel closes.
	usd, calls := m.Total()
	assert.Equal(t, 2, calls)
	assert.InDelta(t, 0.004, usd, 1e-12)
	m.Mark("check")

	assert.Equal(t, "cost $0.0040: review $0.0020, check $0.0020 (prices from 2026-11-09)", m.Summary())
}

func TestMeterSummary(t *testing.T) {
	t.Parallel()

	usage := &llm.Usage{InputTokens: 1_000, OutputTokens: 500, TotalTokens: 1_500}

	m := NewMeter(fakeLLM{usage: usage}, testPricing)
	assert.Empty(t, m.Summary(), "no calls, no cost")
	m.Mark("check")
	assert.Empty(t, m.Summary(), "a step without calls isn't a part")

	_, _ = m.Generate(context.Background(), "s", "p")
	m.Mark("commit")
	assert.Equal(t, "cost $0.0020 (prices from 2026-11-09)", m.Summary(), "one step shows no parts")

	_, _ = m.Generate(context.Background(), "s", "p")
	assert.Equal(t, "cost $0.0040: commit $0.0020, other $0.0020 (prices from 2026-11-09)", m.Summary(),
		"calls after the last mark are shown as other")
}

func TestMeterCountsFailedCallsWithUsage(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	m := NewMeter(fakeLLM{usage: &llm.Usage{InputTokens: 1_000, TotalTokens: 1_000}, err: boom}, testPricing)
	_, err := m.Generate(context.Background(), "s", "p")
	require.ErrorIs(t, err, boom)

	m2 := NewMeter(fakeLLM{err: boom}, testPricing)
	_, _ = m2.Generate(context.Background(), "s", "p")

	_, calls := m.Total()
	assert.Equal(t, 1, calls, "a failed call that reported usage was billed")
	_, calls = m2.Total()
	assert.Equal(t, 0, calls, "a call without usage isn't counted")
}

func TestNilMeter(t *testing.T) {
	t.Parallel()

	var m *Meter
	m.Mark("review")
	assert.Empty(t, m.Summary())
	assert.Nil(t, m.Pricing())
	assert.Nil(t, m.Cost())
	usd, calls := m.Total()
	assert.Zero(t, usd)
	assert.Zero(t, calls)
}

// Cost is Summary's figures as data: the total, each marked step's part
// (and "other" for calls after the last mark), and the prices' date.
func TestMeterCost(t *testing.T) {
	t.Parallel()

	usage := &llm.Usage{InputTokens: 1_000, OutputTokens: 500, TotalTokens: 1_500}
	m := NewMeter(fakeLLM{usage: usage}, testPricing)
	assert.Nil(t, m.Cost(), "no calls, no cost")

	_, _ = m.Generate(context.Background(), "s", "p")
	m.Mark("review")
	_, _ = m.Generate(context.Background(), "s", "p")
	m.Mark("check")
	_, _ = m.Generate(context.Background(), "s", "p")

	assert.Equal(t, &Cost{USD: 0.006, Calls: 3, Steps: []Step{
		{Name: "review", USD: 0.002}, {Name: "check", USD: 0.002}, {Name: "other", USD: 0.002},
	}, PricesDate: "2026-11-09"}, m.Cost())
}

// Sums of float prices are rounded to a millionth of a dollar, so JSON
// shows 0.3, not 0.30000000000000004.
func TestMeterCostRounds(t *testing.T) {
	t.Parallel()

	m := NewMeter(fakeLLM{usage: &llm.Usage{InputTokens: 100_000, TotalTokens: 100_000}}, testPricing)
	for range 3 {
		_, _ = m.Generate(context.Background(), "s", "p")
	}
	m.Mark("review")
	c := m.Cost()
	require.NotNil(t, c)
	assert.Equal(t, 0.3, c.USD, "exact: it is rounded")
	assert.Equal(t, 0.3, c.Steps[0].USD)
}
