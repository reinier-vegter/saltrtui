package ui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestHighstateViewFitsAndKeepsReviewVisible(t *testing.T) {
	v := HighstateViewData{Context: "/etc/salt", ID: "web-01", Presence: "not observed",
		Preview: Source{At: time.Now()}, PreviewReport: HighstateReport{
			Proposed: 1, Steps: []HighstateStep{{ID: "pkg_|-baseline", Status: "proposed", Comment: "Would install", Changes: `{"package":"base"}`}}}, CanApply: true}
	for _, width := range []int{120, 80, 36, 5, 1} {
		for _, height := range []int{25, 9, 4, 2, 1} {
			for _, confirm := range []bool{false, true} {
				v.Width, v.Height, v.Confirm = width, height, confirm
				view := RenderHighstate(v)
				lines := strings.Split(view, "\n")
				if len(lines) != height {
					t.Fatalf("%dx%d: rendered %d rows", width, height, len(lines))
				}
				for _, line := range lines {
					if lipgloss.Width(line) != width {
						t.Fatalf("%dx%d: got %d cells: %q", width, height, lipgloss.Width(line), ansi.Strip(line))
					}
				}
			}
		}
	}
	v.Width, v.Height, v.Confirm = 100, 18, true
	text := ansi.Strip(RenderHighstate(v))
	if !strings.Contains(text, "CONFIRM state.highstate APPLY on web-01") || !strings.Contains(text, "y: apply once") || !strings.Contains(text, "n/esc: cancel") {
		t.Fatalf("confirmation lost exact target or cancellation: %s", text)
	}
	v.Confirm = false
	if text = ansi.Strip(RenderHighstate(v)); !strings.Contains(text, "proposed · pkg_|-baseline") || !strings.Contains(text, "a: review apply") {
		t.Fatalf("review missing result or action: %s", text)
	}
	v.Offset = HighstateScrollLimit(v)
	if text = ansi.Strip(RenderHighstate(v)); !strings.Contains(text, "Preview succeeded. Press a to review Apply") {
		t.Fatalf("last review line cannot be reached: %s", text)
	}
}

func TestHighstateConfirmationAlwaysShowsFactsOrBlocksApply(t *testing.T) {
	for _, id := range []string{"web-01", strings.Repeat("long-minion-", 20)} {
		for _, width := range []int{120, 80, 36, 20, 5} {
			for _, height := range []int{40, 18, 10, 8, 4, 1} {
				v := HighstateViewData{Width: width, Height: height, ID: id, Context: "/etc/salt", Confirm: true, Offset: 999}
				text := ansi.Strip(RenderHighstate(v))
				if HighstateConfirmationFits(v) {
					// Wrapped content may split words; inspect the actual full body
					// capacity as well as the intact dispatch/cancel footer.
					lines := highstateConfirmationLines(v, width-4)
					for _, line := range lines {
						if !strings.Contains(text, ansi.Strip(line)) {
							t.Fatalf("%dx%d: hidden confirmation line %q", width, height, line)
						}
					}
					if !strings.Contains(text, "y: apply once") || !strings.Contains(text, "n/esc: cancel") {
						t.Fatalf("%dx%d: incomplete confirmation controls", width, height)
					}
				} else if strings.Contains(text, "y: apply once") {
					t.Fatalf("%dx%d: unsafe Apply advertised", width, height)
				}
			}
		}
	}
}
