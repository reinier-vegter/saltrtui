package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestActionLayoutCellBudgetAndFallback(t *testing.T) {
	spec := ActionBoxSpec{Scope: "東京-minion (pending)", Reason: "A long status reason wraps within the fixed action box", Alert: true,
		Primary: []hint{{"A", "accept key"}, {"b", "block key"}},
		Nav:     []hint{{"enter", "inspect fingerprint"}, {"/", "search"}, {"r", "refresh"}, {"?", "help"}, {"q", "quit"}}}
	for _, size := range [][2]int{{1, 1}, {20, 3}, {40, 8}, {59, 11}, {60, 12}, {80, 24}, {120, 40}} {
		width, height := size[0], size[1]
		header := 0
		if height > 1 {
			header = len(modeFrame(width, height, "Keys", "/etc/salt", ""))
		}
		layout := LayoutActions(spec, width, height, header)
		if layout.Height != len(layout.Rows) || layout.BodyHeight+layout.Height+header != height {
			t.Fatalf("invalid allocation at %dx%d: %+v header=%d", width, height, layout, header)
		}
		for _, row := range layout.Rows {
			if ansi.StringWidth(row) != width {
				t.Fatalf("row width at %dx%d: got %d: %q", width, height, ansi.StringWidth(row), ansi.Strip(row))
			}
		}
		if layout.Height > 1 {
			if layout.Height > min(6, height/3) || layout.Height > height-header ||
				!strings.Contains(ansi.Strip(layout.Rows[0]), "Actions") || !strings.HasSuffix(ansi.Strip(layout.Rows[len(layout.Rows)-1]), "╯") {
				t.Fatalf("box violated fit rule at %dx%d: %+v", width, height, layout)
			}
		} else if height != 1 && layout.BodyHeight+1+header != height {
			t.Fatalf("fallback did not preserve footer allocation at %dx%d: %+v", width, height, layout)
		}
	}

	for _, width := range []int{20, 40, 59, 60, 80, 120} {
		layout := LayoutActions(spec, width, 40, 2)
		text := ansi.Strip(strings.Join(layout.Rows, "\n"))
		for _, pair := range []string{"A: accept key", "b: block key", "enter: inspect fingerprint", "q: quit"} {
			if !strings.Contains(text, pair) && layout.Height > 1 {
				t.Fatalf("fitting box split or lost %q at width %d: %s", pair, width, text)
			}
		}
	}

	blocked := ActionBoxSpec{Primary: []hint{{"enter", "inspect fingerprint"}}, Nav: []hint{{"q", "quit"}}}
	layout := LayoutActions(blocked, 20, 12, 2)
	if layout.Height != 1 || strings.Contains(ansi.Strip(layout.Rows[0]), "enter: inspect fingerprint") {
		t.Fatalf("unfittable pair did not use action-bar budget: %+v", layout)
	}
	want := actionBar(20, blocked.Reason, blocked.Alert, append(blocked.Primary, blocked.Nav...)...)
	if layout.Rows[0] != want {
		t.Fatal("fallback differs from the existing action bar")
	}
	if len(LayoutActions(ActionBoxSpec{}, 11, 12, 2).Rows) != 1 {
		t.Fatal("inner widths below eight cells must fall back")
	}
	if len(LayoutActions(ActionBoxSpec{}, 0, 0, 0).Rows) != 0 {
		t.Fatal("zero dimensions rendered action content")
	}
	keysView := KeysViewData{Width: 120, Height: 40}
	keysLayout := keysBottomLayout(keysView)
	if KeysListRows(keysView) != max(1, keysLayout.BodyHeight-4) || KeysPageRows(keysView) != max(1, keysLayout.BodyHeight-3) {
		t.Fatal("Keys capacities do not follow the shared bottom layout")
	}
	highstateView := HighstateViewData{Width: 120, Height: 40, ID: "web-01"}
	if HighstatePageRows(highstateView) != max(1, highstateBottomLayout(highstateView).BodyHeight-3) {
		t.Fatal("Highstate page capacity does not follow the shared bottom layout")
	}
}

func TestPackHintsKeepsUnicodePairsWhole(t *testing.T) {
	hints := []hint{{"↑↓", "scroll"}, {"enter", "inspect fingerprint"}, {"q", "quit"}}
	rows, ok := packHints(hints, 30)
	if !ok || len(rows) < 2 {
		t.Fatalf("expected wrapped complete hints: %v %v", rows, ok)
	}
	text := ansi.Strip(strings.Join(rows, "\n"))
	for _, h := range hints {
		if !strings.Contains(text, h.key+": "+h.label) {
			t.Fatalf("packed hint split: %q missing from %q", h.key+": "+h.label, text)
		}
	}
	for _, row := range rows {
		if ansi.StringWidth(row) > 30 {
			t.Fatalf("packed row exceeds cell budget: %q", row)
		}
	}
	if _, ok := packHints([]hint{{"enter", "inspect fingerprint"}}, 12); ok {
		t.Fatal("overwide key/action pair was accepted")
	}
}

func TestActionBoxScreensMatchSharedLayoutAtApprovedSizes(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {20, 3}, {40, 8}, {59, 11}, {60, 12}, {80, 24}, {120, 40}} {
		width, height := size[0], size[1]
		keysView := KeysViewData{Width: width, Height: height, Context: "/etc/salt", BackendAvailable: true,
			Selected: KeyRow{ID: "東京-01", State: "pending"}, Rows: []KeyRow{{ID: "東京-01", State: "pending", Selected: true}}, List: Source{At: time.Now()}}
		highstateView := HighstateViewData{Width: width, Height: height, Context: "/etc/salt", ID: "web-01", CanPreview: true}
		updateView := UpdateViewData{Width: width, Height: height, Context: "/etc/salt", Running: "v1.2.3", Available: "v1.2.4", Phase: "downloading"}
		cases := []struct {
			name, view string
			layout     PromptLayout
			header     int
		}{
			{"Keys", RenderKeys(keysView), keysBottomLayout(keysView), actionHeaderRows(width, height, "Keys", "/etc/salt", "")},
			{"Highstate", RenderHighstate(highstateView), highstateBottomLayout(highstateView), actionHeaderRows(width, height, "Fleet", "/etc/salt", "Highstate · web-01")},
			{"Update", RenderSelfUpdate(updateView), updateBottomLayout(updateView), actionHeaderRows(width, height, "Fleet", "/etc/salt", "Update")},
		}
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s-%dx%d", tc.name, width, height), func(t *testing.T) {
				rows := strings.Split(tc.view, "\n")
				if len(rows) != height || tc.layout.Height != len(tc.layout.Rows) ||
					tc.layout.BodyHeight+tc.layout.Height+tc.header != height ||
					len(rows)-tc.layout.Height-tc.header != tc.layout.BodyHeight {
					t.Fatalf("screen/layout allocation mismatch: %+v header=%d rows=%d", tc.layout, tc.header, len(rows))
				}
				for _, row := range rows {
					if ansi.StringWidth(row) != width {
						t.Fatalf("row width %d, want %d: %q", ansi.StringWidth(row), width, ansi.Strip(row))
					}
				}
				if !strings.HasSuffix(tc.view, strings.Join(tc.layout.Rows, "\n")) {
					t.Fatal("rendered bottom rows differ from shared layout")
				}
			})
		}
	}
}

func actionHeaderRows(width, height int, mode, context, subview string) int {
	if height <= 1 {
		return 0
	}
	return len(modeFrame(width, height, mode, context, subview))
}

func TestKeyActionsAndPanelsAcrossStates(t *testing.T) {
	base := KeysViewData{Width: 100, Height: 24, Context: "/etc/salt", Focus: 0, BackendAvailable: true,
		Selected: KeyRow{ID: "web-01", State: "pending"}, Rows: []KeyRow{{ID: "web-01", State: "pending", Selected: true}},
		List: Source{At: time.Now()}}
	cases := []struct {
		name        string
		change      func(*KeysViewData)
		want, avoid []string
	}{
		{"pending", func(v *KeysViewData) {}, []string{"A: accept key", "b: block key", "enter: inspect fingerprint"}, []string{"d: deny/revoke key"}},
		{"accepted", func(v *KeysViewData) { v.Selected.State, v.Rows[0].State = "accepted", "accepted" }, []string{"d: deny/revoke key", "Accepted: this minion is authorized on the"}, []string{"A: accept key", "b: block key"}},
		{"rejected", func(v *KeysViewData) { v.Selected.State, v.Rows[0].State = "rejected", "rejected" }, []string{"No key action is available for a rejected key"}, []string{"A: accept key", "b: block key", "d: deny/revoke key"}},
		{"denied", func(v *KeysViewData) { v.Selected.State, v.Rows[0].State = "denied", "denied" }, []string{"No key action is available for a denied key"}, []string{"A: accept key", "b: block key", "d: deny/revoke key"}},
		{"ambiguous", func(v *KeysViewData) { v.Ambiguous = true }, []string{"Key actions are disabled: this ID occurs in multiple key states"}, []string{"A: accept key", "b: block key", "d: deny/revoke key"}},
		{"busy", func(v *KeysViewData) { v.Busy = true }, []string{"Key action in progress; wait for refresh"}, []string{"A: accept key", "b: block key", "r: refresh"}},
		{"error", func(v *KeysViewData) { v.List.Err = errors.New("inventory denied") }, []string{"Keys unavailable: inventory denied", "r: retry"}, []string{"A: accept key", "b: block key", "r: refresh"}},
		{"searching", func(v *KeysViewData) { v.Searching, v.Search = true, "/ web" }, []string{"Filtering keys", "enter/esc: close search"}, []string{"A: accept key", "b: block key", "r: refresh"}},
		{"empty-search", func(v *KeysViewData) { v.Selected, v.Rows, v.Query = KeyRow{}, nil, "missing" }, []string{"No matching keys"}, []string{"A: accept key", "b: block key", "d: deny/revoke key"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := base
			v.Rows = append([]KeyRow(nil), base.Rows...)
			tc.change(&v)
			layout := keysBottomLayout(v)
			view := RenderKeys(v)
			panelText := actionPanelText(view, layout, len(modeFrame(v.Width, v.Height, "Keys", v.Context, "")))
			assertNoPanelInstructions(t, panelText)
			all := ansi.Strip(view)
			for _, text := range tc.want {
				if !strings.Contains(all, text) {
					t.Errorf("missing %q: %s", text, all)
				}
			}
			for _, text := range tc.avoid {
				if strings.Contains(all, text) {
					t.Errorf("unexpected %q: %s", text, all)
				}
			}
		})
	}
	if text := ansi.Strip(RenderKeys(KeysViewData{Width: 100, Height: 24, List: Source{Err: errors.New("offline")}})); !strings.Contains(text, "Keys unavailable") {
		t.Fatal("failed empty state is not descriptive")
	}
	withoutBackend := base
	withoutBackend.BackendAvailable = false
	noBackendText := ansi.Strip(RenderKeys(withoutBackend))
	for _, unavailable := range []string{"A: accept key", "b: block key", "enter: inspect fingerprint", "r: refresh"} {
		if strings.Contains(noBackendText, unavailable) {
			t.Errorf("unavailable key action advertised: %s", unavailable)
		}
	}
}

func TestHighstateActionsAndPanelsAcrossStates(t *testing.T) {
	now := time.Now()
	base := HighstateViewData{Width: 100, Height: 24, Context: "/etc/salt", ID: "web-01", Presence: "connected", CanPreview: true,
		Preview: Source{At: now}, PreviewReport: HighstateReport{Proposed: 1, Steps: []HighstateStep{{ID: "state", Status: "proposed"}}}}
	cases := []struct {
		name   string
		change func(*HighstateViewData)
		want   []string
	}{
		{"loading", func(v *HighstateViewData) { v.Busy, v.Preview = true, Source{Busy: true}; v.Status = "Preview running" }, []string{"Preview running", "esc: wait for result"}},
		{"empty", func(v *HighstateViewData) { v.Preview, v.PreviewReport = Source{}, HighstateReport{} }, []string{"No preview has run for this minion.", "p: preview"}},
		{"failed", func(v *HighstateViewData) { v.PreviewReport.Failed, v.CanApply = 2, false }, []string{"This preview cannot be applied.", "Apply blocked: preview reported 2 failed states (see results above)"}},
		{"ready", func(v *HighstateViewData) { v.CanApply = true }, []string{"Preview succeeded: no failed states.", "a: review Apply"}},
		{"expired", func(v *HighstateViewData) { v.Expired, v.CanApply = true, false }, []string{"Preview expired.", "Apply blocked: preview expired; run a new preview"}},
		{"applied", func(v *HighstateViewData) { v.Attempted, v.CanApply, v.Apply = true, false, Source{Busy: true} }, []string{"Apply already sent for this preview.", "Apply blocked: run a new preview before another Apply"}},
		{"error", func(v *HighstateViewData) { v.Preview, v.CanApply = Source{Err: errors.New("offline")}, false }, []string{"Preview unavailable.", "Apply blocked: preview unavailable"}},
		{"empty-result", func(v *HighstateViewData) { v.PreviewReport, v.CanApply = HighstateReport{}, false }, []string{"This preview cannot be applied.", "Apply blocked: preview returned no state results"}},
		{"eligibility", func(v *HighstateViewData) { v.CanApply = false }, []string{"This preview cannot be applied.", "Apply blocked: selection changed or the key is no longer accepted"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := base
			tc.change(&v)
			layout := highstateBottomLayout(v)
			view := RenderHighstate(v)
			panelText := actionPanelText(view, layout, len(modeFrame(v.Width, v.Height, "Fleet", v.Context, "Highstate · "+v.ID)))
			assertNoPanelInstructions(t, panelText)
			all := ansi.Strip(strings.Join(layout.Rows, "\n"))
			for _, text := range tc.want {
				if !strings.Contains(all, text) && !strings.Contains(ansi.Strip(view), text) {
					t.Errorf("missing %q: %s", text, ansi.Strip(view))
				}
			}
		})
	}
	noPreview := base
	noPreview.CanPreview = false
	noPreview.Status = "Highstate unavailable; no gateway configured"
	if text := ansi.Strip(RenderHighstate(noPreview)); strings.Contains(text, "p: preview") {
		t.Fatal("preview advertised without a gateway")
	}
}

func TestUpdateActionBoxPhasesAndPanels(t *testing.T) {
	for _, phase := range []string{"inspecting", "confirm", "downloading", "verifying", "installing", "done", "failed"} {
		t.Run(phase, func(t *testing.T) {
			v := UpdateViewData{Width: 100, Height: 24, Context: "/etc/salt", Running: "v1.2.3", Available: "v1.2.4",
				Current: "/usr/local/bin/saltrtui", Phase: phase, Text: "State for " + phase}
			var layout PromptLayout
			var header int
			if phase == "confirm" {
				layout = UpdatePromptLayout(v)
				header = len(decisionFrame(v.Width, v.Height, "Fleet", v.Context, "Update"))
			} else {
				layout = updateBottomLayout(v)
				header = len(modeFrame(v.Width, v.Height, "Fleet", v.Context, "Update"))
			}
			view := RenderSelfUpdate(v)
			panelText := actionPanelText(view, layout, header)
			assertNoPanelInstructions(t, panelText)
			box := ansi.Strip(strings.Join(layout.Rows, "\n"))
			switch phase {
			case "confirm":
				if !strings.Contains(box, "Confirm in-place update") || !strings.Contains(box, "enter: confirm choice") {
					t.Fatal("RFC 0028 confirmation changed")
				}
			case "downloading", "verifying":
				if !strings.Contains(box, "esc: cancel") || !strings.Contains(box, "q: quit") {
					t.Fatalf("missing cancellation/quit actions: %s", box)
				}
			case "installing":
				if !strings.Contains(box, "Installing the verified release; this cannot be interrupted") || strings.Contains(box, "esc:") || strings.Contains(box, "q:") {
					t.Fatalf("installing actions are inaccurate: %s", box)
				}
			default:
				if !strings.Contains(box, "esc: back") || !strings.Contains(box, "q: quit") {
					t.Fatalf("missing back/quit actions: %s", box)
				}
			}
		})
	}
}

func TestOtherScreenPanelsContainStateNotActionInstructions(t *testing.T) {
	views := []struct {
		name, view, want, footer string
	}{
		{"Fleet", Render(ViewData{Width: 120, Height: 24, Inventory: Source{Err: errors.New("offline")}}), "Inventory unavailable", "r: retry"},
		{"Events", RenderEvents(EventsViewData{Width: 120, Height: 24, Family: "all", Status: "offline"}), "Listener unavailable", "r: restart"},
		{"Jobs", RenderJobs(JobsViewData{Width: 120, Height: 24, List: Source{Err: errors.New("offline")}}), "Jobs unavailable", "r: retry"},
		{"Target", RenderTarget(TargetViewData{Width: 120, Height: 24, Input: "web-*"}), "No current target preview is available.", "Ctrl+P: preview"},
	}
	for _, tc := range views {
		t.Run(tc.name, func(t *testing.T) {
			all := ansi.Strip(tc.view)
			assertNoPanelInstructions(t, all[:strings.LastIndex(all, "\n")])
			if !strings.Contains(all, tc.want) {
				t.Fatalf("missing state %q: %s", tc.want, all)
			}
			if !strings.Contains(all, tc.footer) {
				t.Fatalf("footer lost recovery/preview action %q: %s", tc.footer, all)
			}
		})
	}
	if text := ansi.Strip(RenderEvents(EventsViewData{Width: 120, Height: 24, Family: "all", Query: "missing"})); !strings.Contains(text, "No matching buffered events") {
		t.Fatal("event filter empty state is missing")
	}
	if text := ansi.Strip(RenderJobs(JobsViewData{Width: 120, Height: 24, Query: "missing", List: Source{At: time.Now()}})); !strings.Contains(text, "No matching jobs") {
		t.Fatal("job filter empty state is missing")
	}
	extra := []struct {
		name, view, want string
	}{
		{"Fleet filtered", Render(ViewData{Width: 120, Height: 24, Query: "missing", Search: "/ missing", Inventory: Source{At: time.Now()}}), "No matching IDs"},
		{"Fleet presence", Render(ViewData{Width: 120, Height: 24, Presence: Source{Err: errors.New("offline")}}), "Presence unavailable"},
		{"Fleet empty detail", Render(ViewData{Width: 120, Height: 24}), "No minion selected."},
		{"Fleet grains", Render(ViewData{Width: 120, Height: 24, Focus: 2, Selected: "web-01", Rows: []Row{{ID: "web-01", Selected: true}}}), "Grains not fetched yet."},
		{"Events detail", RenderEvents(EventsViewData{Width: 120, Height: 24, Family: "all"}), "No event selected."},
		{"Jobs returns", RenderJobs(JobsViewData{Width: 120, Height: 24, Selected: "jid", Items: []JobSummary{{JID: "jid"}}}), "Returns not loaded yet."},
		{"Target suggestions", RenderTarget(TargetViewData{Width: 120, Height: 24, Input: "web"}), "No suggestions for this prefix"},
	}
	for _, tc := range extra {
		t.Run(tc.name, func(t *testing.T) {
			all := ansi.Strip(tc.view)
			assertNoPanelInstructions(t, all[:strings.LastIndex(all, "\n")])
			if !strings.Contains(all, tc.want) {
				t.Fatalf("missing state %q: %s", tc.want, all)
			}
		})
	}
	inspectFooters := []struct {
		name, view string
	}{
		{"Fleet", Render(ViewData{Width: 120, Height: 24, Selected: "web-01", Rows: []Row{{ID: "web-01", Selected: true}}})},
		{"Events", RenderEvents(EventsViewData{Width: 120, Height: 24, Family: "all", Selected: 0, Rows: []EventRow{{Tag: "event"}}})},
		{"Jobs", RenderJobs(JobsViewData{Width: 120, Height: 24, Selected: "jid", Items: []JobSummary{{JID: "jid"}}})},
	}
	for _, tc := range inspectFooters {
		lines := strings.Split(ansi.Strip(tc.view), "\n")
		footer := lines[len(lines)-1]
		if !strings.Contains(footer, "enter: inspect") {
			t.Errorf("%s footer lost inspection action: %s", tc.name, footer)
		}
	}
}

func actionPanelText(view string, layout PromptLayout, headerRows int) string {
	rows := strings.Split(ansi.Strip(view), "\n")
	end := len(rows) - layout.Height
	if headerRows > end {
		return ""
	}
	return strings.Join(rows[headerRows:end], "\n")
}

func assertNoPanelInstructions(t *testing.T, text string) {
	t.Helper()
	lower := strings.ToLower(text)
	for _, instruction := range []string{"press ", "enter to", "to retry", "to inspect", "clear search", "clear the search", "clear it to", "preview again", "retry with", "review every matched"} {
		if strings.Contains(lower, instruction) {
			t.Errorf("panel contains action instruction %q: %s", instruction, text)
		}
	}
}

func TestLongScopeWrapsBelowActionsTitle(t *testing.T) {
	scope := fmt.Sprintf("%s\x1b[31m", strings.Repeat("東京-minion-", 8))
	spec := ActionBoxSpec{Scope: scope, Primary: []hint{{"p", "preview"}}, Nav: []hint{{"q", "quit"}}}
	layout := LayoutActions(spec, 120, 40, 2)
	if layout.Height <= 1 {
		t.Fatalf("long scope should fit as wrapped box content: %+v", layout)
	}
	text := ansi.Strip(strings.Join(layout.Rows, "\n"))
	if !strings.Contains(text, "Actions") || !strings.Contains(text, "Scope:") || strings.Contains(text, "\x1b") {
		t.Fatalf("scope was clipped or treated as ANSI: %s", text)
	}
}
