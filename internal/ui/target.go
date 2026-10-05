package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type TargetViewData struct {
	Context, Mode, Expression, Input, Error, Notice string
	Width, Height, Offset, Cursor, Suggestion       int
	ResultsFocus, Busy, Stale, Saved                bool
	Suggestions, IDs                                []string
	Inventory                                       Source
	Preview                                         Source
	Presence                                        Source
	Connected                                       map[string]bool
}

func targetBuilderLines(v TargetViewData, width int) []string {
	mode := "Specific minions · comma-separated accepted IDs"
	if v.Mode == "glob" {
		mode = "Minion ID pattern · * means any characters; ? means one"
	}
	lines := []string{
		"Master config: " + v.Context,
		"Matcher: " + mode,
		"Example: web-01,web-02 (specific)  or  web-* (pattern)",
		"Target: " + ansi.Strip(v.Input),
		"Ctrl+T matcher · Ctrl+P preview · Ctrl+S save · Ctrl+N results · Esc close",
		"Suggestions from accepted keys (Tab completes an ID):",
	}
	if len(v.Suggestions) == 0 {
		lines = append(lines, "  Type an ID prefix for suggestions")
	}
	for i, suggestion := range v.Suggestions {
		marker := "  "
		if i == v.Suggestion {
			marker = "> "
		}
		lines = append(lines, marker+suggestion)
	}
	lines = append(lines, "", "Candidate preview · matches among accepted minion keys")
	if v.Busy {
		lines = append(lines, "Fetching fresh accepted keys…")
	}
	if v.Preview.Err != nil {
		lines = append(lines, "Preview failed: "+clean(v.Preview.Err.Error()))
		if !v.Preview.At.IsZero() {
			lines = append(lines, "Previous candidate snapshot is stale")
		}
	}
	if v.Error != "" {
		lines = append(lines, "Cannot preview: "+v.Error)
	}
	if v.Stale && !v.Preview.At.IsZero() {
		lines = append(lines, "Draft changed; preview again before saving")
	}
	if !v.Preview.At.IsZero() {
		lines = append(lines, "Effective "+v.Mode+": "+v.Expression,
			fmt.Sprintf("%d accepted-key matches · %s", len(v.IDs), stamp(v.Preview)))
		if len(v.IDs) == 0 {
			lines = append(lines, "No accepted minions match; edit the target and preview again")
		}
		if len(v.IDs) >= 10 {
			lines = append(lines, "Broad scope: review every matched ID before saving")
		}
		lines = append(lines, "Presence is separate from targeting; a match may not respond.")
		for i, id := range v.IDs {
			status := "unknown"
			if !v.Presence.At.IsZero() && !v.Presence.Busy && v.Presence.Err == nil {
				status = "not observed"
				if v.Connected[id] {
					status = "connected"
				}
			}
			marker := "  "
			if v.ResultsFocus && i == v.Cursor {
				marker = "> "
			}
			lines = append(lines, marker+id+" · "+status)
		}
	} else if !v.Busy && v.Error == "" {
		lines = append(lines, "Press Ctrl+P to preview before saving")
	}
	if v.Notice != "" {
		lines = append(lines, "", v.Notice)
	}
	if v.Saved {
		lines = append(lines, "", "Reviewed target saved in memory for future actions")
	}
	return styledLines(lines, width)
}

func TargetScrollLimit(v TargetViewData) int {
	return max(0, len(targetBuilderLines(v, max(1, v.Width-4)))-max(1, v.Height-6))
}

// Keep the highlighted preview row inside the bounded content viewport.
func TargetCursorOffset(v TargetViewData) int {
	if !v.ResultsFocus || v.Cursor < 0 || v.Cursor >= len(v.IDs) {
		return min(v.Offset, TargetScrollLimit(v))
	}
	lines := targetBuilderLines(v, max(1, v.Width-4))
	want := "> " + v.IDs[v.Cursor] + " · "
	for i, line := range lines {
		if strings.HasPrefix(ansi.Strip(line), want) {
			rows := max(1, v.Height-6)
			if i < v.Offset {
				return i
			}
			if i >= v.Offset+rows {
				return min(i-rows+1, TargetScrollLimit(v))
			}
			break
		}
	}
	return min(v.Offset, TargetScrollLimit(v))
}

func RenderTarget(v TargetViewData) string {
	w, h := max(0, v.Width), max(0, v.Height)
	if w == 0 || h == 0 {
		return ""
	}
	hints := []hint{{"Ctrl+T", "type"}, {"Ctrl+P", "preview"}, {"Ctrl+S", "save"}, {"Tab", "complete"}, {"Ctrl+N", "results"}, {"Esc", "Fleet"}}
	notice := ""
	if v.ResultsFocus {
		hints = []hint{{"↑↓", "select"}, {"x", "remove"}, {"Tab", "edit"}, {"Ctrl+P", "preview"}, {"Ctrl+S", "save"}, {"Esc", "Fleet"}}
	}
	if v.Saved {
		hints, notice = []hint{{"Esc", "Fleet"}}, "Target saved in memory · actions not available yet"
	}
	if h == 1 {
		return actionBar(w, notice, false, hints...)
	}
	frame := []string{modeBar(w, "Fleet", v.Context, "Target builder")}
	if h >= 4 {
		frame = append(frame, muted.Render(strings.Repeat("─", w)))
	}
	if bodyHeight := h - len(frame) - 1; bodyHeight > 0 {
		body := panel("Accepted-key target preview", targetBuilderLines(v, max(1, w-4)), v.Offset, w, bodyHeight, true, nil)
		frame = append(frame, strings.Split(body, "\n")...)
	}
	frame = append(frame, actionBar(w, notice, false, hints...))
	return strings.Join(frame, "\n")
}
