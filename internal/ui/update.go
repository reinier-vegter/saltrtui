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

func UpdatePrompt(v UpdateViewData) PromptSpec {
	privileges := "no sudo required"
	if !v.Writable {
		privileges = "sudo required; the system terminal owns authentication"
	}
	options := []string{"  Update in place", "  Cancel"}
	if v.Index >= 0 && v.Index < len(options) {
		options[v.Index] = "> " + strings.TrimSpace(options[v.Index])
	}
	return PromptSpec{Title: "Confirm in-place update", Facts: []string{
		"Version: " + promptValue(v.Running) + " -> " + promptValue(v.Available),
		"Current location: " + promptValue(v.Current), "Destination: " + promptValue(v.Current),
		"Privileges: " + privileges,
	}, Consequence: "Download, verify, then replace this executable in place.", Options: options,
		Controls: []hint{{"up/down", "choose"}, {"enter", "confirm choice"}, {"esc", "back"}, {"q", "quit"}},
		Fallback: []string{"esc: back · resize", "esc: back", "q: quit", "q"}, MinWidth: 60, MinHeight: 12}
}

func UpdatePromptLayout(v UpdateViewData) PromptLayout {
	return LayoutPrompt(UpdatePrompt(v), v.Width, v.Height,
		len(decisionFrame(v.Width, v.Height, "Fleet", v.Context, "Update")))
}

func updateActions(v UpdateViewData) ActionBoxSpec {
	spec := ActionBoxSpec{}
	if v.Available != "" {
		spec.Scope = v.Running + " -> " + v.Available
	}
	switch v.Phase {
	case "downloading", "verifying":
		spec.Primary = []hint{{"esc", "cancel"}}
		spec.Nav = []hint{{"q", "quit"}}
	case "installing":
		spec.Reason = "Installing the verified release; this cannot be interrupted"
	default:
		spec.Primary = []hint{{"esc", "back"}}
		spec.Nav = []hint{{"q", "quit"}}
	}
	return spec
}

func updateBottomLayout(v UpdateViewData) PromptLayout {
	if v.Phase == "confirm" {
		return UpdatePromptLayout(v)
	}
	w, h := max(0, v.Width), max(0, v.Height)
	headerRows := 0
	if h > 1 {
		headerRows = len(modeFrame(w, h, "Fleet", v.Context, "Update"))
	}
	return LayoutActions(updateActions(v), w, h, headerRows)
}

func updateLines(v UpdateViewData, width int) []string {
	lines := []string{"Update saltrtui", ""}
	if v.Phase == "confirm" {
		lines = append(lines, "Release details", "https://github.com/reinier-vegter/saltrtui/releases",
			"Restart after installation to use the new version.")
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
	capacity := max(0, updateBottomLayout(v).BodyHeight-3)
	return max(0, len(updateLines(v, v.Width-4))-capacity)
}

func RenderSelfUpdate(v UpdateViewData) string {
	w, h := max(0, v.Width), max(0, v.Height)
	if w == 0 || h == 0 {
		return ""
	}
	if v.Phase == "confirm" {
		frame := decisionFrame(w, h, "Fleet", v.Context, "Update")
		layout := UpdatePromptLayout(v)
		if layout.BodyHeight > 0 {
			frame = append(frame, strings.Split(panel("Update saltrtui", updateLines(v, w-4), 0, w, layout.BodyHeight, false, nil), "\n")...)
		}
		return strings.Join(append(frame, layout.Rows...), "\n")
	}
	if h == 1 {
		return strings.Join(updateBottomLayout(v).Rows, "\n")
	}
	frame := modeFrame(w, h, "Fleet", v.Context, "Update")
	layout := updateBottomLayout(v)
	bodyHeight := layout.BodyHeight
	if bodyHeight > 0 {
		body := panel("Update saltrtui", updateLines(v, w-4), 0, w, bodyHeight, true, nil)
		frame = append(frame, strings.Split(body, "\n")...)
	}
	frame = append(frame, layout.Rows...)
	return strings.Join(frame, "\n")
}
