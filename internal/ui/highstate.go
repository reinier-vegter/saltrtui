package ui

import (
	"fmt"
	"strings"
)

type HighstateStep struct {
	ID, Status, Comment, Changes string
}

type HighstateReport struct {
	Steps                        []HighstateStep
	Proposed, Changed, Unchanged int
	Failed                       int
}

type HighstateViewData struct {
	Width, Height, Offset int
	Context, ID, Presence string
	Status                string
	Preview, Apply        Source
	PreviewReport         HighstateReport
	ApplyReport           HighstateReport
	Confirm, Busy, Help   bool
	CanApply, Attempted   bool
	Expired               bool
}

func highstateReportLines(title string, report HighstateReport) []string {
	lines := []string{title + ": " + fmt.Sprintf("%d proposed · %d changed · %d unchanged · %d failed", report.Proposed, report.Changed, report.Unchanged, report.Failed)}
	for _, step := range report.Steps {
		lines = append(lines, step.Status+" · "+step.ID)
		if step.Comment != "" {
			lines = append(lines, "  "+step.Comment)
		}
		if step.Changes != "" && step.Changes != "{}" {
			lines = append(lines, "  Changes: "+step.Changes)
		}
	}
	return lines
}

func highstateLines(v HighstateViewData, width int) []string {
	if v.Confirm {
		if !HighstateConfirmationFits(v) {
			return styledLines([]string{"Resize to review Apply; n/esc cancels."}, width)
		}
		return highstateConfirmationLines(v, width)
	}
	lines := []string{
		"Minion: " + v.ID,
		"Master config: " + v.Context,
		"Presence: " + v.Presence + " (observation, not job success)",
		"Scope: one accepted minion · its configured highstate",
		"No Salt environment or pillar override", "",
	}
	if v.Help {
		lines = append(lines, "p preview (sends a Salt read/test job)", "a review Apply after a successful preview",
			"y confirm Apply · n/esc cancel", "esc return to Fleet when idle",
			"CLI timeouts do not prove a remote state run stopped.",
			"Results can contain sensitive state changes; no automatic retries.")
		return styledLines(lines, width)
	}
	lines = append(lines, sourceLine("Preview", v.Preview))
	if v.Preview.Busy {
		lines = append(lines, "Preview job running; wait for the minion return.")
	} else if v.Preview.At.IsZero() && v.Preview.Err == nil {
		lines = append(lines, "Press p to preview the selected minion's highstate.")
	} else if v.Preview.Err != nil {
		lines = append(lines, "Preview unavailable; Apply is disabled.")
	} else {
		lines = append(lines, highstateReportLines("Preview results", v.PreviewReport)...)
		if v.CanApply {
			lines = append(lines, "", "Preview succeeded. Press a to review Apply.")
		} else if v.Expired {
			lines = append(lines, "", "Preview expired; press p for a new review.")
		} else if v.Attempted {
			lines = append(lines, "", "A new preview is needed before another Apply.")
		} else {
			lines = append(lines, "", "Apply disabled; review failed states or changed selection.")
		}
	}
	if v.Attempted {
		lines = append(lines, "", sourceLine("Apply", v.Apply))
		if v.Apply.Busy {
			lines = append(lines, "Apply job dispatched; do not repeat it.")
		} else if v.Apply.Err == nil && !v.Apply.At.IsZero() {
			lines = append(lines, highstateReportLines("Apply results", v.ApplyReport)...)
		}
	}
	if v.Status != "" {
		lines = append(lines, "", "Status: "+v.Status)
	}
	return styledLines(lines, width)
}

func highstateConfirmationLines(v HighstateViewData, width int) []string {
	return styledLines([]string{
		"CONFIRM state.highstate APPLY on " + v.ID,
		"Master config: " + v.Context,
		"This can change the minion; the accepted key is rechecked first.",
		"Press y to dispatch once, n/esc to cancel.",
	}, width)
}

// HighstateConfirmationFits is shared by rendering and dispatch. Confirmation
// must expose the entire target/consequence and both intact footer controls.
func HighstateConfirmationFits(v HighstateViewData) bool {
	return v.Width >= len("y: apply once  n/esc: cancel")+3 && v.Height >= 7 &&
		len(highstateConfirmationLines(v, v.Width-4)) <= v.Height-6
}

func HighstateScrollLimit(v HighstateViewData) int {
	if v.Width < 6 || v.Height < 6 {
		return 0
	}
	return max(0, len(highstateLines(v, v.Width-4))-PageRows(v.Height, 1, false))
}

func RenderHighstate(v HighstateViewData) string {
	w, h := max(0, v.Width), max(0, v.Height)
	if w == 0 || h == 0 {
		return ""
	}
	hints := []hint{{"p", "preview"}, {"a", "review apply"}, {"↑↓", "scroll"}, {"esc", "Fleet"}, {"?", "help"}}
	if v.Confirm {
		hints = []hint{{"y", "apply once"}, {"n/esc", "cancel"}}
		if !HighstateConfirmationFits(v) {
			hints = []hint{{"n/esc", "cancel"}}
		}
	} else if v.Busy {
		hints = []hint{{"↑↓", "scroll"}, {"esc", "wait for result"}}
	} else if v.Help {
		hints = []hint{{"↑↓", "scroll"}, {"?", "return"}, {"esc", "Fleet"}}
	} else if !v.CanApply {
		hints = []hint{{"p", "preview"}, {"↑↓", "scroll"}, {"esc", "Fleet"}, {"?", "help"}}
	}
	notice := v.Status
	if v.Confirm {
		// Confirmation facts own the body; diagnostics cannot crowd its controls.
		notice = ""
		if !HighstateConfirmationFits(v) {
			notice = "Resize to review Apply"
		}
	}
	alert := notice != "" && !v.Busy
	if h == 1 {
		return actionBar(w, notice, alert, hints...)
	}
	frame := modeFrame(w, h, "Fleet", v.Context, "Highstate · "+v.ID)
	if bodyHeight := h - len(frame) - 1; bodyHeight > 0 {
		name := "Highstate · one minion"
		if v.Confirm {
			name = "Confirm Apply · " + v.ID
		} else if v.Help {
			name = "Highstate help"
		}
		offset := v.Offset
		if v.Confirm {
			offset = 0
		}
		frame = append(frame, strings.Split(panel(name, highstateLines(v, max(0, w-4)), offset, w, bodyHeight, true, nil), "\n")...)
	}
	frame = append(frame, actionBar(w, notice, alert, hints...))
	return strings.Join(frame, "\n")
}
