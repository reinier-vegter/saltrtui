package ui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// PromptSpec is presentation only. Eligibility and dispatch belong to app.
type PromptSpec struct {
	Title, Consequence  string
	Facts, Options      []string
	Controls            []hint
	Fallback            []string
	MinWidth, MinHeight int
}

type PromptLayout struct {
	Rows               []string
	Height, BodyHeight int
	FitsConfirmation   bool
}

type ActionBoxSpec struct {
	Scope, Reason string
	Alert         bool
	Primary, Nav  []hint
}

// LayoutPrompt is the sole fit calculation used by both rendering and dispatch.
// It never clips mandatory facts or splits a key/action pair. The body may be
// empty, but the header and the complete confirmation must fit together.
func LayoutPrompt(spec PromptSpec, width, height, headerRows int) PromptLayout {
	if width <= 0 || height <= 0 {
		return PromptLayout{}
	}
	available := max(0, height-headerRows)
	inner := width - 4
	if inner > 0 {
		lines := styledLines([]string{"▸ " + ansi.Strip(spec.Title)}, inner)
		for _, fact := range spec.Facts {
			lines = append(lines, styledLines([]string{ansi.Strip(fact)}, inner)...)
		}
		lines = append(lines, styledLines([]string{ansi.Strip(spec.Consequence)}, inner)...)
		for _, option := range spec.Options {
			lines = append(lines, styledLines([]string{ansi.Strip(option)}, inner)...)
		}
		controlsFit := len(spec.Controls) > 0
		for _, line := range lines {
			if ansi.StringWidth(line) > inner {
				controlsFit = false
			}
		}
		var controls string
		for _, control := range spec.Controls {
			plain := control.key + ": " + control.label
			if ansi.StringWidth(plain) > inner {
				controlsFit = false
				break
			}
			if controls != "" && ansi.StringWidth(controls)+2+ansi.StringWidth(plain) > inner {
				lines = append(lines, controls)
				controls = ""
			}
			if controls != "" {
				controls += "  "
			}
			controls += keycap.Render(control.key) + item.Render(": "+control.label)
		}
		if controls != "" {
			lines = append(lines, controls)
		}
		rows := []string{noticeStyle.Render("╭" + strings.Repeat("─", width-2) + "╮")}
		for _, line := range lines {
			rows = append(rows, noticeStyle.Render("│")+" "+fixed(line, inner)+" "+noticeStyle.Render("│"))
		}
		rows = append(rows, noticeStyle.Render("╰"+strings.Repeat("─", width-2)+"╯"))
		if controlsFit && width >= spec.MinWidth && height >= spec.MinHeight && len(rows) <= available {
			return PromptLayout{Rows: rows, Height: len(rows), BodyHeight: available - len(rows), FitsConfirmation: true}
		}
	}
	// A full header is sacrificed before the only safe exit at one-row sizes.
	for _, variant := range spec.Fallback {
		if ansi.StringWidth(variant) <= width {
			return PromptLayout{Rows: []string{fixed(item.Render(variant), width)}, Height: 1, BodyHeight: max(0, available-1)}
		}
	}
	return PromptLayout{Rows: []string{strings.Repeat(" ", width)}, Height: 1, BodyHeight: max(0, available-1)}
}

var cancelPromptFallback = []string{"n/esc: cancel · resize", "n: cancel", "n"}

// LayoutActions keeps informational actions in a fixed bottom box only when
// its complete content fits; otherwise it returns the existing one-row bar.
func LayoutActions(spec ActionBoxSpec, width, height, headerRows int) PromptLayout {
	if width <= 0 || height <= 0 {
		return PromptLayout{}
	}

	available := max(0, height-headerRows)
	hints := append(append([]hint(nil), spec.Primary...), spec.Nav...)
	fallback := func() PromptLayout {
		row := actionBar(width, spec.Reason, spec.Alert, hints...)
		return PromptLayout{Rows: []string{row}, Height: 1, BodyHeight: max(0, available-1)}
	}
	inner := width - 4
	if inner < 8 || height == 1 {
		return fallback()
	}

	scope := promptValue(spec.Scope)
	title := "Actions"
	var content []string
	if scope != "" {
		candidate := title + " · " + scope
		if ansi.StringWidth(candidate)+5 <= width {
			title = candidate
		} else {
			content = append(content, actionBoxWrapped("Scope: "+scope, inner, muted)...)
		}
	}
	if reason := strings.TrimSpace(ansi.Strip(spec.Reason)); reason != "" {
		style := muted
		if spec.Alert {
			style = failure
		}
		content = append(content, actionBoxWrapped(reason, inner, style)...)
	}
	primary, ok := packHints(spec.Primary, inner)
	if !ok {
		return fallback()
	}
	content = append(content, primary...)
	nav, ok := packHints(spec.Nav, inner)
	if !ok {
		return fallback()
	}
	content = append(content, nav...)

	titleWidth := ansi.StringWidth(title)
	if titleWidth+5 > width {
		return fallback()
	}
	top := accent.Render("╭─ ") + accent.Render(title) + accent.Render(strings.Repeat("─", width-titleWidth-4)) + accent.Render("╮")
	rows := []string{top}
	for _, line := range content {
		rows = append(rows, accent.Render("│")+" "+fixed(line, inner)+" "+accent.Render("│"))
	}
	rows = append(rows, accent.Render("╰"+strings.Repeat("─", width-2)+"╯"))
	if len(rows) > min(6, height/3) || len(rows) > available {
		return fallback()
	}
	return PromptLayout{Rows: rows, Height: len(rows), BodyHeight: available - len(rows)}
}

func actionBoxWrapped(text string, width int, style lipgloss.Style) []string {
	if width <= 0 {
		return nil
	}
	wrapped := ansi.Hardwrap(ansi.Wordwrap(clean(text), width, " /,="), width, false)
	lines := strings.Split(wrapped, "\n")
	for i, line := range lines {
		lines[i] = style.Render(line)
	}
	return lines
}

// packHints wraps complete key/action pairs by display-cell width.
func packHints(hints []hint, width int) ([]string, bool) {
	if width <= 0 {
		return nil, len(hints) == 0
	}
	var rows []string
	line, cells := "", 0
	for _, h := range hints {
		plain := h.key + ": " + h.label
		pairWidth := ansi.StringWidth(plain)
		if pairWidth > width {
			return nil, false
		}
		styled := keycap.Render(h.key) + item.Render(": "+h.label)
		if line != "" && cells+2+pairWidth > width {
			rows = append(rows, line)
			line, cells = "", 0
		}
		if line != "" {
			line += "  "
			cells += 2
		}
		line += styled
		cells += pairWidth
	}
	if line != "" {
		rows = append(rows, line)
	}
	return rows, true
}

// Preserve exact scope even for filesystem paths or IDs with non-printable
// characters. They are displayed escaped, never silently stripped/replaced.
func promptValue(value string) string {
	if clean(value) != value || strings.ContainsAny(value, "\\\"") {
		return strconv.Quote(value)
	}
	return value
}

// decisionFrame retains current-page identity but removes tab mnemonics while
// the prompt owns input. It reserves the bottom safe exit even on tiny screens.
func decisionFrame(width, height int, mode, context, subview string) []string {
	frame := modeFrame(width, height, mode, context, subview, true)
	return frame[:min(len(frame), max(0, height-1))]
}
