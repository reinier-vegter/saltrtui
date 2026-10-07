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
	activeMode = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
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

type modeEntry struct{ name, short, key string }

var workbenchModes = []modeEntry{{"Fleet", "F", "f"}, {"Jobs", "J", "j"}, {"Assignments", "A", "a"}, {"Events", "E", "e"}, {"Keys", "K", "k"}}

// NavigationMinimumWidth is the smallest framed width that can show every
// compact workspace mnemonic without clipping a tab.
func NavigationMinimumWidth() int {
	return 3*len(workbenchModes) + 2*(len(workbenchModes)-1)
}

func renderModeLabel(name string, active bool) string {
	style := item
	if active {
		style = activeMode
	}
	if name == "" {
		return ""
	}
	return style.Render(" ") + style.Underline(true).Render(name[:1]) + style.Render(name[1:]+" ")
}

// modeFrame keeps the same order and position even in nested views. It measures
// only unstyled text, then styles complete spans so ANSI sequences never affect
// layout or truncate a box-drawing rail.
func modeFrame(width, height int, mode, context, subview string) []string {
	if width <= 0 || height <= 0 {
		return nil
	}
	if width < NavigationMinimumWidth() {
		return []string{padHeader(muted.Render(clip(" Resize terminal", width)), width)}
	}
	compact := width < 49
	showBrand := !compact && width >= 62
	plain := ""
	parts := make([]string, 0, len(workbenchModes)+2)
	if showBrand {
		plain += " saltrtui │  "
		parts = append(parts, accent.Render(" saltrtui ")+muted.Render("│"))
	}
	activeStart, activeEnd := 0, 0
	for i, entry := range workbenchModes {
		if i > 0 {
			plain += "  "
		}
		name := entry.name
		if compact {
			name = entry.short
		}
		start := ansi.StringWidth(plain)
		plain += " " + name + " "
		if entry.name == mode {
			activeStart, activeEnd = start, ansi.StringWidth(plain)
		}
		parts = append(parts, renderModeLabel(name, entry.name == mode))
	}
	body := strings.Join(parts, "  ")
	if subview != "" && width-ansi.StringWidth(plain) >= ansi.StringWidth("  / "+subview) {
		plain += "  / " + subview
		body += accent.Render("  / " + subview)
	}
	contextPrefix := "  · master config: "
	if context != "" && width-ansi.StringWidth(plain) >= ansi.StringWidth(contextPrefix)+8 {
		available := width - ansi.StringWidth(plain) - ansi.StringWidth(contextPrefix)
		label := clip(context, available)
		plain += contextPrefix + label
		body += muted.Render(contextPrefix + label)
	}
	frame := []string{padHeader(body, width)}
	if height < 4 {
		return frame
	}
	if activeEnd > activeStart && activeStart < width {
		activeEnd = min(activeEnd, width)
		rail := muted.Render(strings.Repeat("─", activeStart)) + accent.Render(strings.Repeat("━", activeEnd-activeStart)) + muted.Render(strings.Repeat("─", width-activeEnd))
		return append(frame, rail)
	}
	return append(frame, muted.Render(strings.Repeat("─", width)))
}

func padHeader(body string, width int) string {
	return body + strings.Repeat(" ", max(0, width-ansi.StringWidth(body)))
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
