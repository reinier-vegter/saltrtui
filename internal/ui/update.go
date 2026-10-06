package ui

import "strings"

// UpdateViewData describes the dedicated self-update review/progress screen.
// U opens it; Enter only ever starts a download from the "confirm" phase.
type UpdateViewData struct {
	Width, Height, Index        int
	Context, Running, Available string
	Phase, Text                 string
	Current                     string
	Writable                    bool
}

func updateConfirmLines(v UpdateViewData, width int) []string {
	if v.Width < 60 || v.Height < 12 {
		return []string{"Resize the terminal to review the update destination before confirming.", "", "Esc: back"}
	}
	transition := v.Running + " -> " + v.Available
	lines := []string{transition, "", "Current location", v.Current, ""}
	var options, details []string
	options = []string{"Update in place", "Cancel"}
	if v.Writable {
		details = []string{v.Current + " · no sudo", ""}
	} else {
		lines = append(lines, "This location requires administrator privileges.", "")
		details = []string{v.Current + " · sudo required", ""}
	}
	for index, option := range options {
		cursor := "  "
		if index == v.Index {
			cursor = "> "
		}
		lines = append(lines, cursor+option)
		if details[index] != "" {
			lines = append(lines, "    "+details[index])
		}
	}
	lines = append(lines, "", "Release details", "https://github.com/reinier-vegter/saltrtui/releases")
	return lines
}

func updateLines(v UpdateViewData, width int) []string {
	lines := []string{"Update saltrtui", ""}
	if v.Phase == "confirm" {
		lines = append(lines, updateConfirmLines(v, width)...)
		return styledLines(lines, width)
	}
	text := v.Text
	if text == "" {
		text = "Preparing…"
	}
	lines = append(lines, strings.Split(text, "\n")...)
	return styledLines(lines, width)
}

// UpdateScrollLimit gives the model the actual end of the rendered screen.
func UpdateScrollLimit(v UpdateViewData) int {
	if v.Width < 6 || v.Height < 6 {
		return 0
	}
	return max(0, len(updateLines(v, v.Width-4))-PageRows(v.Height, 1, false))
}

func RenderSelfUpdate(v UpdateViewData) string {
	w, h := max(0, v.Width), max(0, v.Height)
	if w == 0 || h == 0 {
		return ""
	}
	var hints []hint
	switch v.Phase {
	case "confirm":
		hints = []hint{{"up/down", "choose"}, {"enter", "confirm"}, {"esc", "back"}, {"q", "quit"}}
	case "downloading", "verifying":
		hints = []hint{{"esc", "cancel"}, {"q", "quit"}}
	case "installing":
		hints = nil
	default:
		hints = []hint{{"esc", "back"}, {"q", "quit"}}
	}
	if h == 1 {
		return actionBar(w, "", false, hints...)
	}
	frame := []string{modeBar(w, "Fleet", v.Context, "Update")}
	if h >= 4 {
		frame = append(frame, muted.Render(strings.Repeat("─", w)))
	}
	bodyHeight := h - len(frame) - 1
	if bodyHeight > 0 {
		body := panel("Update saltrtui", updateLines(v, w-4), 0, w, bodyHeight, true, nil)
		frame = append(frame, strings.Split(body, "\n")...)
	}
	frame = append(frame, actionBar(w, "", false, hints...))
	return strings.Join(frame, "\n")
}
