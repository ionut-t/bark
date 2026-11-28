package prices

import (
	"context"
	"math"
	"strings"
	"sync"

	"github.com/ionut-t/bark/v2/internal/llm"
)

// Meter is an llm.LLM that adds up what the calls made through it cost.
type Meter struct {
	llm.LLM
	pricing *Pricing

	mu               sync.Mutex
	calls, marked    int
	total, markedUSD float64
	parts            []part
}

// NewMeter wraps client so the cost of its calls is counted at pricing.
func NewMeter(client llm.LLM, pricing *Pricing) *Meter {
	return &Meter{LLM: client, pricing: pricing}
}

// Pricing is the prices the meter counts with; nil for a nil Meter.
func (m *Meter) Pricing() *Pricing {
	if m == nil {
		return nil
	}
	return m.pricing
}

// Generate makes the call and counts its cost.
func (m *Meter) Generate(ctx context.Context, system, prompt string) (llm.Response, error) {
	resp, err := m.LLM.Generate(ctx, system, prompt)
	m.add(resp.Usage)
	return resp, err
}

// Stream passes the stream on and counts its cost when it ends, before the
// response channel is closed.
func (m *Meter) Stream(ctx context.Context, system, prompt string) (<-chan llm.Response, <-chan error) {
	in, errs := m.LLM.Stream(ctx, system, prompt)
	out := make(chan llm.Response)
	go func() {
		defer close(out)
		var usage *llm.Usage
		defer func() { m.add(usage) }()
		for r := range in {
			if r.Usage != nil {
				usage = r.Usage
			}
			select {
			case out <- r:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, errs
}

func (m *Meter) add(u *llm.Usage) {
	if u == nil {
		return
	}
	cost := m.pricing.Cost(*u)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.total += cost
}

// part is the cost of the calls one step of a command made.
type part struct {
	label string
	usd   float64
}

// Mark names the calls made since the last Mark (e.g. "review", "check")
// for Summary. Without calls since, it does nothing. A nil Meter does
// nothing.
func (m *Meter) Mark(label string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.calls == m.marked {
		return
	}
	m.parts = append(m.parts, part{label: label, usd: m.total - m.markedUSD})
	m.marked, m.markedUSD = m.calls, m.total
}

// Total returns the cost of every call so far, and how many there were. A
// nil Meter has none.
func (m *Meter) Total() (usd float64, calls int) {
	if m == nil {
		return 0, 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.total, m.calls
}

// Summary describes what the calls cost, with each marked step's part when
// there is more than one, and the prices' date:
//
//	cost $0.0234: review $0.0123, check $0.0111 (prices from 2026-11-09)
//
// It is empty without calls, or for a nil Meter.
func (m *Meter) Summary() string {
	c := m.Cost()
	if c == nil {
		return ""
	}
	s := "cost " + FormatUSD(c.USD)
	if len(c.Steps) > 1 {
		items := make([]string, len(c.Steps))
		for i, p := range c.Steps {
			items[i] = p.Name + " " + FormatUSD(p.USD)
		}
		s += ": " + strings.Join(items, ", ")
	}
	return s + " (prices from " + c.PricesDate + ")"
}

// Cost is what a command's model calls cost, as `bark review --format json`
// reports it.
type Cost struct {
	// USD is the cost of every call, in US dollars.
	USD   float64 `json:"usd"`
	Calls int     `json:"calls"`
	// Steps are the parts of USD each step of the command cost, in order:
	// "review", "check", "verdict rewrite", and "other" for calls no step
	// was marked for.
	Steps []Step `json:"steps"`
	// PricesDate is the date of the prices the cost was counted with.
	PricesDate string `json:"pricesDate"`
}

// Step is what one step of a command cost.
type Step struct {
	Name string  `json:"name"`
	USD  float64 `json:"usd"`
}

// Cost returns what the calls cost so far, with each marked step's part
// (see Mark), rounded to a millionth of a dollar. It is nil without calls,
// or for a nil Meter.
func (m *Meter) Cost() *Cost {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.calls == 0 {
		return nil
	}
	steps := make([]Step, 0, len(m.parts)+1)
	for _, p := range m.parts {
		steps = append(steps, Step{Name: p.label, USD: roundUSD(p.usd)})
	}
	if m.calls > m.marked {
		steps = append(steps, Step{Name: "other", USD: roundUSD(m.total - m.markedUSD)})
	}
	return &Cost{USD: roundUSD(m.total), Calls: m.calls, Steps: steps, PricesDate: m.pricing.Date}
}

// roundUSD rounds usd to a millionth of a dollar, so sums of float prices
// don't show as 0.30000000000000004.
func roundUSD(usd float64) float64 {
	return math.Round(usd*1e6) / 1e6
}
