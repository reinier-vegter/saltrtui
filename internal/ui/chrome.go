package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The chrome occupies the same header and footer rows in every view. Keep
// navigation separate from actions so an error cannot hide the mode map.
type hint struct{ key, label string }

var (
	chromeBG   = lipgloss.Color("235")
	chromeEdge = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	keycap     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117")).Background(lipgloss.Color("238"))
	activeMode = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("81"))
)

func chromeLine(width int, body string) string {
	if width <= 0 {
		return ""
	}
	if width < 4 {
		return fixed(body, width)
	}
	inner := width - 2
	return chromeEdge.Render("│") + lipgloss.NewStyle().Background(chromeBG).Render(fixed(body, inner)) + chromeEdge.Render("│")
}

// modeBar keeps the same order and position even in nested views. Numbers in
// secondary views are landmarks; Esc returns to Fleet before mode switching.
func modeBar(width int, mode, context, subview string) string {
	if width <= 0 {
		return ""
	}
	var parts []string
	if width >= 60 {
		parts = append(parts, accent.Render(" saltrtui ")+muted.Render("│"))
	}
	for _, entry := range []struct{ name, key string }{{"Fleet", "1"}, {"Jobs", "2"}, {"Events", "4"}, {"Keys", "6"}} {
		text := entry.key + " " + entry.name
		if entry.name == mode {
			parts = append(parts, activeMode.Render(text))
		} else {
			parts = append(parts, item.Render(text))
		}
	}
	body := " " + strings.Join(parts, "  ")
	if subview != "" && width >= 70 {
		body += accent.Render("  / " + subview)
	}
	if context != "" && width >= 104 {
		body += muted.Render("  ·  master config: " + clip(context, max(0, width-ansi.StringWidth(body)-22)))
	}
	return chromeLine(width, body)
}

func actionBar(width int, notice string, alert bool, hints ...hint) string {
	if width <= 0 {
		return ""
	}
	// Keep the recovery action and some of the diagnostic legible on a narrow
	// terminal instead of clipping the entire diagnostic behind optional hints.
	if alert && notice != "" && width < 70 && len(hints) > 1 {
		hints = hints[:1]
	}
	parts := make([]string, 0, len(hints)+1)
	for _, h := range hints {
		parts = append(parts, keycap.Render(h.key)+item.Render(" "+h.label))
	}
	if notice != "" {
		label := "  │  " + clean(notice)
		if alert {
			parts = append(parts, failure.Render(label))
		} else {
			parts = append(parts, muted.Render(label))
		}
	}
	return chromeLine(width, " "+strings.Join(parts, "  "))
}
