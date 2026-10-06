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
	for _, entry := range []struct{ name, short, key string }{{"Fleet", "F", "1"}, {"Jobs", "J", "2"}, {"Assignments", "A", "3"}, {"Events", "E", "4"}, {"Keys", "K", "6"}} {
		name := entry.name
		if width < 70 {
			name = entry.short
			if width >= 36 && entry.name == "Fleet" {
				name = entry.name
			}
		}
		text := entry.key + " " + name
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
	budget := width - 3 // frame edges and initial space
	if notice != "" {
		budget -= min(30, max(0, budget/3))
	}
	chosen := make([]bool, len(hints))
	used := 0
	choose := func(i int) {
		if chosen[i] {
			return
		}
		cells := ansi.StringWidth(hints[i].key + ": " + hints[i].label)
		if used > 0 {
			cells += 2
		}
		if used+cells <= budget {
			chosen[i], used = true, used+cells
		}
	}
	// Recovery and safe exits must survive ahead of optional task shortcuts.
	if alert && len(hints) > 0 {
		choose(0)
	}
	for i, h := range hints {
		if strings.Contains(h.key, "esc") || strings.Contains(h.key, "Esc") || strings.Contains(h.key, "?") || h.key == "q" || h.key == "ctrl+c" {
			choose(i)
		}
	}
	for i := range hints {
		choose(i)
	}
	parts := make([]string, 0, len(hints)+1)
	for i, h := range hints {
		if chosen[i] {
			parts = append(parts, keycap.Render(h.key)+item.Render(": "+h.label))
		}
	}
	if len(parts) == 0 && len(hints) > 0 {
		return chromeLine(width, " Resize")
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
