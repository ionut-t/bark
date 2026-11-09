package prices

import (
	"context"
	"slices"
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
	if m == nil {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.calls == 0 {
		return ""
	}
	parts := m.parts
	if m.calls > m.marked {
		parts = append(slices.Clip(parts), part{label: "other", usd: m.total - m.markedUSD})
	}
	s := "cost " + FormatUSD(m.total)
	if len(parts) > 1 {
		items := make([]string, len(parts))
		for i, p := range parts {
			items[i] = p.label + " " + FormatUSD(p.usd)
		}
		s += ": " + strings.Join(items, ", ")
	}
	return s + " (prices from " + m.pricing.Date + ")"
}
