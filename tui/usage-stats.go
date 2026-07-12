package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/ionut-t/bark/v2/internal/llm"
	"github.com/ionut-t/coffee/styles"
)

type usageStats struct {
	usage    llm.Usage
	provider string
	model    string
	duration time.Duration
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

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		"",
		row("Provider:", stats.provider),
		row("Model:", stats.model),
		row("Input:", formatTokens(stats.usage.InputTokens)+" tokens"),
		row("Output:", formatTokens(stats.usage.OutputTokens)+" tokens"),
		row("Total:", formatTokens(stats.usage.TotalTokens)+" tokens"),
		row("Duration:", formatDuration(d)),
		"",
		s.Subtext0.Render("esc/ctrl+t to close"),
	)

	return lipgloss.NewStyle().
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.Primary.GetForeground()).
		Render(content)
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
