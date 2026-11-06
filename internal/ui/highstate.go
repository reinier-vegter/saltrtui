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
	CanPreview            bool
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
		lines = append(lines, "Preview running; awaiting the minion return.")
	} else if v.Preview.At.IsZero() && v.Preview.Err == nil {
		lines = append(lines, "No preview has run for this minion.")
	} else if v.Preview.Err != nil {
		lines = append(lines, "Preview unavailable.")
	} else {
		lines = append(lines, highstateReportLines("Preview results", v.PreviewReport)...)
		if v.Confirm {
			// Approval and controls belong exclusively to the bottom box.
		} else if v.CanApply {
			lines = append(lines, "", "Preview succeeded: no failed states.")
		} else if v.Expired {
			lines = append(lines, "", "Preview expired.")
		} else if v.Attempted {
			lines = append(lines, "", "Apply already sent for this preview.")
		} else {
			lines = append(lines, "", "This preview cannot be applied.")
		}
	}
	if v.Attempted {
		lines = append(lines, "", sourceLine("Apply", v.Apply))
		if !v.Apply.Busy && v.Apply.Err == nil && !v.Apply.At.IsZero() {
			lines = append(lines, highstateReportLines("Apply results", v.ApplyReport)...)
		}
	}
	return styledLines(lines, width)
}

func HighstatePrompt(v HighstateViewData) PromptSpec {
	return PromptSpec{Title: "Confirm Apply · state.highstate", Facts: []string{
		"Minion: " + promptValue(v.ID), "Master config: " + promptValue(v.Context), "Action: state.highstate test=False",
	}, Consequence: "This can change the minion; the accepted key is rechecked first.",
		Controls: []hint{{"y", "apply once"}, {"n/esc", "cancel"}}, Fallback: cancelPromptFallback}
}

func HighstatePromptLayout(v HighstateViewData) PromptLayout {
	return LayoutPrompt(HighstatePrompt(v), v.Width, v.Height,
		len(decisionFrame(v.Width, v.Height, "Fleet", v.Context, "Highstate · "+v.ID)))
}

// HighstateConfirmationFits is shared by rendering and dispatch. Confirmation
// must expose the entire target/consequence and both intact footer controls.
func HighstateConfirmationFits(v HighstateViewData) bool {
	return HighstatePromptLayout(v).FitsConfirmation
}

func highstateBlockedReason(v HighstateViewData) string {
	if v.Status != "" {
		return v.Status
	}
	if v.Preview.Busy || (v.Preview.At.IsZero() && v.Preview.Err == nil) || v.CanApply {
		return ""
	}
	switch {
	case v.Preview.Err != nil:
		return "Apply blocked: preview unavailable"
	case v.Attempted:
		return "Apply blocked: run a new preview before another Apply"
	case v.Expired:
		return "Apply blocked: preview expired; run a new preview"
	case v.PreviewReport.Failed > 0:
		return fmt.Sprintf("Apply blocked: preview reported %d failed states (see results above)", v.PreviewReport.Failed)
	case len(v.PreviewReport.Steps) == 0:
		return "Apply blocked: preview returned no state results"
	default:
		return "Apply blocked: selection changed or the key is no longer accepted"
	}
}

func highstateActions(v HighstateViewData) ActionBoxSpec {
	spec := ActionBoxSpec{Scope: v.ID}
	spec.Reason = highstateBlockedReason(v)
	status := strings.ToLower(v.Status)
	spec.Alert = spec.Reason != "" && !v.Busy && (v.Preview.Err != nil || v.Apply.Err != nil || v.Expired ||
		v.PreviewReport.Failed > 0 || v.ApplyReport.Failed > 0 ||
		v.Status == "" && !v.CanApply && (!v.Preview.At.IsZero() || v.Preview.Err != nil) ||
		strings.Contains(status, "unavailable") || strings.Contains(status, "failed") || strings.Contains(status, "expired") || strings.Contains(status, "unknown"))
	if v.Help {
		spec.Nav = []hint{{"↑↓", "scroll"}, {"?", "return"}, {"esc", "Fleet"}}
		return spec
	}
	if v.Busy {
		spec.Nav = []hint{{"↑↓", "scroll"}, {"esc", "wait for result"}}
		return spec
	}
	if v.CanPreview {
		spec.Primary = []hint{{"p", "preview"}}
	}
	if v.CanApply {
		spec.Primary = append(spec.Primary, hint{"a", "review Apply"})
	}
	spec.Nav = []hint{{"↑↓", "scroll"}, {"esc", "Fleet"}, {"?", "help"}}
	return spec
}

func highstateBottomLayout(v HighstateViewData) PromptLayout {
	if v.Confirm {
		return HighstatePromptLayout(v)
	}
	w, h := max(0, v.Width), max(0, v.Height)
	headerRows := 0
	if h > 1 {
		headerRows = len(modeFrame(w, h, "Fleet", v.Context, "Highstate · "+v.ID))
	}
	return LayoutActions(highstateActions(v), w, h, headerRows)
}

func HighstatePageRows(v HighstateViewData) int {
	return max(1, highstateBottomLayout(v).BodyHeight-3)
}

func HighstateScrollLimit(v HighstateViewData) int {
	if v.Width < 6 || v.Height < 6 {
		return 0
	}
	capacity := max(0, highstateBottomLayout(v).BodyHeight-3)
	return max(0, len(highstateLines(v, v.Width-4))-capacity)
}

func RenderHighstate(v HighstateViewData) string {
	w, h := max(0, v.Width), max(0, v.Height)
	if w == 0 || h == 0 {
		return ""
	}
	if v.Confirm {
		frame := decisionFrame(w, h, "Fleet", v.Context, "Highstate · "+v.ID)
		layout := HighstatePromptLayout(v)
		if layout.BodyHeight > 0 {
			frame = append(frame, strings.Split(panel("Highstate · one minion", highstateLines(v, w-4), v.Offset, w, layout.BodyHeight, false, nil), "\n")...)
		}
		return strings.Join(append(frame, layout.Rows...), "\n")
	}
	if h == 1 {
		return strings.Join(highstateBottomLayout(v).Rows, "\n")
	}
	frame := modeFrame(w, h, "Fleet", v.Context, "Highstate · "+v.ID)
	layout := highstateBottomLayout(v)
	if bodyHeight := layout.BodyHeight; bodyHeight > 0 {
		name := "Highstate · one minion"
		if v.Help {
			name = "Highstate help"
		}
		offset := v.Offset
		frame = append(frame, strings.Split(panel(name, highstateLines(v, max(0, w-4)), offset, w, bodyHeight, true, nil), "\n")...)
	}
	frame = append(frame, layout.Rows...)
	return strings.Join(frame, "\n")
}
