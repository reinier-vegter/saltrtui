package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestSharedNavigationAndActionFrame(t *testing.T) {
	views := []struct {
		name, active string
		render       func(int, int) string
	}{
		{"Fleet", "Fleet", func(w, h int) string { return Render(ViewData{Width: w, Height: h}) }},
		{"Jobs", "Jobs", func(w, h int) string { return RenderJobs(JobsViewData{Width: w, Height: h}) }},
		{"Assignments", "Assignments", func(w, h int) string { return RenderAssignments(AssignmentsViewData{Width: w, Height: h}) }},
		{"Events", "Events", func(w, h int) string { return RenderEvents(EventsViewData{Width: w, Height: h}) }},
		{"Keys", "Keys", func(w, h int) string { return RenderKeys(KeysViewData{Width: w, Height: h}) }},
		{"Console", "Fleet", func(w, h int) string { return RenderConsole(ConsoleViewData{Width: w, Height: h}) }},
		{"Target", "Fleet", func(w, h int) string { return RenderTarget(TargetViewData{Width: w, Height: h}) }},
		{"Highstate", "Fleet", func(w, h int) string { return RenderHighstate(HighstateViewData{Width: w, Height: h}) }},
		{"Update", "Fleet", func(w, h int) string { return RenderSelfUpdate(UpdateViewData{Width: w, Height: h}) }},
	}
	for _, view := range views {
		for _, w := range []int{120, 80, 42, NavigationMinimumWidth(), NavigationMinimumWidth() - 1, 5, 1} {
			for _, h := range []int{20, 8, 4, 2, 1} {
				t.Run(view.name, func(t *testing.T) {
					lines := strings.Split(view.render(w, h), "\n")
					if len(lines) != h {
						t.Fatalf("%dx%d: %d lines", w, h, len(lines))
					}
					for _, line := range lines {
						if lipgloss.Width(line) != w {
							t.Fatalf("%dx%d: row is %d cells: %q", w, h, lipgloss.Width(line), ansi.Strip(line))
						}
					}
					if w >= NavigationMinimumWidth() && h >= 2 {
						header := ansi.Strip(lines[0])
						last := -1
						modes := []string{"Fleet", "Jobs", "Assignments", "Events", "Keys"}
						if w < 51 {
							modes = []string{"F", "J", "A", "E", "K"}
						}
						for _, mode := range modes {
							pos := strings.Index(header, mode)
							if pos <= last {
								t.Fatalf("missing/unsorted modes in %q", header)
							}
							last = pos
						}
						if !strings.Contains(lines[0], "\x1b[") || !strings.Contains(ansi.Strip(lines[h-1]), "│") {
							t.Fatalf("unframed or unstyled chrome: %q / %q", header, ansi.Strip(lines[h-1]))
						}
						if h >= 4 && !strings.Contains(ansi.Strip(lines[1]), "━") {
							t.Fatalf("active tab rail missing: %q", ansi.Strip(lines[1]))
						}
					} else if w >= 8 && w < NavigationMinimumWidth() && h >= 2 && !strings.Contains(ansi.Strip(lines[0]), "Resize") {
						t.Fatalf("missing below-minimum resize state: %q", ansi.Strip(lines[0]))
					}
				})
			}
		}
	}
}

func TestPresenceErrorKeepsModesAndRetry(t *testing.T) {
	v := ViewData{Width: 120, Height: 12, Presence: Source{Err: errors.New("presence: Salt emitted diagnostics on stderr; check Salt logs")}}
	lines := strings.Split(ansi.Strip(Render(v)), "\n")
	if !strings.Contains(lines[0], "Keys") || !strings.Contains(lines[len(lines)-1], "r: retry") ||
		!strings.Contains(lines[len(lines)-1], "Presence: Salt emitted diagnostics") || strings.Contains(lines[len(lines)-1], "Presence: presence:") {
		t.Fatalf("presence failure hid navigation or duplicated source: %q / %q", lines[0], lines[len(lines)-1])
	}
	if !strings.Contains(ansi.Strip(Render(ViewData{Width: 120, Height: 12, Inventory: Source{At: time.Now()}})), "No accepted keys · k Keys") {
		t.Fatal("empty accepted inventory did not direct operator to Keys")
	}
	v.Width = 36
	footer := ansi.Strip(strings.Split(Render(v), "\n")[v.Height-1])
	if !strings.Contains(footer, "r: retry") || !strings.Contains(footer, "Presence") {
		t.Fatalf("narrow error hid recovery or diagnostic: %q", footer)
	}
}

func TestModeFrameAlignsOneActiveRailAndNeverClipsTabs(t *testing.T) {
	for _, mode := range []string{"Fleet", "Jobs", "Assignments", "Events", "Keys"} {
		frame := modeFrame(120, 12, mode, "", "")
		rail := ansi.Strip(frame[1])
		if strings.Count(rail, "━") != len(mode)+2 || strings.Count(rail, "━") == 0 {
			t.Fatalf("%s rail does not span its padded label: %q", mode, rail)
		}
	}
	for _, width := range []int{NavigationMinimumWidth(), NavigationMinimumWidth() - 1} {
		frame := modeFrame(width, 8, "Fleet", "", "")
		header := ansi.Strip(frame[0])
		if width == NavigationMinimumWidth() {
			for _, tab := range []string{"F", "J", "A", "E", "K"} {
				if !strings.Contains(header, tab) {
					t.Fatalf("minimum navigation width hid %q: %q", tab, header)
				}
			}
		} else if !strings.Contains(header, "Resize") {
			t.Fatalf("below minimum did not replace tabs with resize guidance: %q", header)
		}
	}
}

func TestKeyConfirmationRetainsActions(t *testing.T) {
	v := KeysViewData{Width: 100, Height: 12, Confirm: "accept", Selected: KeyRow{ID: "pending-1", State: "pending"}}
	lines := strings.Split(ansi.Strip(RenderKeys(v)), "\n")
	if !strings.Contains(lines[0], "Keys") || !strings.Contains(lines[len(lines)-1], "y: confirm") || !strings.Contains(lines[len(lines)-1], "n/esc: cancel") || !strings.Contains(lines[len(lines)-1], "pending-1") {
		t.Fatalf("confirmation chrome incomplete: %q / %q", lines[0], lines[len(lines)-1])
	}
}

func TestActionBarKeepsWholePairsAndSafeRoutes(t *testing.T) {
	hints := []hint{{"h", "highstate"}, {"s", "console"}, {"g", "graph"}, {"t", "targets"}, {"r", "refresh"}, {"?", "help"}, {"q", "quit"}}
	for _, width := range []int{120, 80, 36, 20} {
		text := strings.TrimSpace(strings.Trim(ansi.Strip(actionBar(width, "", false, hints...)), "│"))
		for _, pair := range strings.Split(text, "  ") {
			valid := false
			for _, h := range hints {
				valid = valid || pair == h.key+": "+h.label
			}
			if !valid {
				t.Fatalf("width %d: partial pair %q", width, pair)
			}
		}
		if !strings.Contains(text, "?: help") || !strings.Contains(text, "q: quit") {
			t.Fatalf("width %d: safe routes hidden: %q", width, text)
		}
	}
	if text := ansi.Strip(actionBar(10, "", false, hint{"long", "unfittable action"})); !strings.Contains(text, "Resize") {
		t.Fatalf("missing resize hint: %q", text)
	}
}

func TestKeysOnlyAdvertisesAvailableMutations(t *testing.T) {
	for _, state := range []string{"pending", "accepted", "rejected", "denied", ""} {
		for _, blocked := range []bool{false, true} {
			v := KeysViewData{Width: 160, Height: 12, Selected: KeyRow{ID: "key", State: state}, Ambiguous: blocked}
			footer := ansi.Strip(strings.Split(RenderKeys(v), "\n")[v.Height-1])
			if strings.Contains(footer, "A: accept") != (state == "pending" && !blocked) ||
				strings.Contains(footer, "b: block") != (state == "pending" && !blocked) ||
				strings.Contains(footer, "d: deny/revoke") != (state == "accepted" && !blocked) {
				t.Fatalf("%s blocked=%v: %q", state, blocked, footer)
			}
		}
	}
}
