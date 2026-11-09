package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/ionut-t/bark/v2/internal/prices"
	"github.com/ionut-t/coffee/styles"
)

type usageStats struct {
	usage    llm.Usage
	provider string
	model    string
	duration time.Duration
	// check is the usage of the check that followed a review, or nil.
	check *llm.Usage
	// pricing, if not nil, adds the calls' cost.
	pricing *prices.Pricing
}

func overlayCenter(base, modal string, width, height int) string {
	// Clamp the modal so it can never widen the compositor canvas past the
	// terminal, which would cause line wrapping on narrow windows.
	modal = lipgloss.NewStyle().MaxWidth(width).MaxHeight(height).Render(modal)

	x := max(0, (width-lipgloss.Width(modal))/2)
	y := max(0, (height-lipgloss.Height(modal))/2)
	c := lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(modal).X(x).Y(y).Z(1),
	)
	return c.Render()
}

func renderUsageStats(stats *usageStats, s styles.Styles) string {
	if stats == nil {
		content := s.Text.Render("No LLM usage recorded yet.")
		content += "\n\n" + s.Subtext0.Render("esc/ctrl+t to close")

		return lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(s.Primary.GetForeground()).
			Render(content)
	}

	title := s.Primary.Bold(true).Render("LLM Usage")

	// Calculate rounded duration (to nearest 10ms)
	d := stats.duration.Round(10 * time.Millisecond)

	labelStyle := s.Subtext0.Width(15)
	valueStyle := s.Text

	row := func(label, value string) string {
		return lipgloss.JoinHorizontal(lipgloss.Top, labelStyle.Render(label), valueStyle.Render(value))
	}

	rows := []string{
		title,
		"",
		row("Provider:", stats.provider),
		row("Model:", stats.model),
		row("Input:", formatTokens(stats.usage.InputTokens)+" tokens"),
	}
	if cached := stats.usage.CachedInputTokens; cached > 0 {
		rows = append(rows, row("Cached:", formatTokens(cached)+" tokens"))
	}
	rows = append(rows,
		row("Output:", formatTokens(stats.usage.OutputTokens)+" tokens"),
		row("Total:", formatTokens(stats.usage.TotalTokens)+" tokens"),
		row("Duration:", formatDuration(d)),
	)
	if c := stats.check; c != nil {
		rows = append(rows, row("Check:", formatTokens(c.InputTokens)+" input, "+formatTokens(c.OutputTokens)+" output tokens"))
	}
	if p := stats.pricing; p != nil {
		rows = append(rows, row("Cost:", costSummary(stats, p)))
	}
	rows = append(rows, "", s.Subtext0.Render("esc/ctrl+t to close"))
	content := lipgloss.JoinVertical(lipgloss.Left, rows...)

	return lipgloss.NewStyle().
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.Primary.GetForeground()).
		Render(content)
}

// costSummary is the cost of the call, or of a review and its check with
// each part.
func costSummary(stats *usageStats, p *prices.Pricing) string {
	from := " (prices from " + p.Date + ")"
	review := p.Cost(stats.usage)
	if stats.check == nil {
		return prices.FormatUSD(review) + from
	}
	check := p.Cost(*stats.check)
	return prices.FormatUSD(review+check) + ": review " + prices.FormatUSD(review) + ", check " + prices.FormatUSD(check) + from
}

func formatTokens(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		b.WriteString(s[:lead])
	}
	for i := lead; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
}
