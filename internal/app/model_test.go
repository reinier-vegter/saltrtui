package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"saltrtui/internal/assignments"
	"saltrtui/internal/fleet"
	"saltrtui/internal/ui"
)

type fakeGateway struct{}

func (fakeGateway) ListAcceptedKeys(context.Context) ([]string, error) {
	return []string{"web-01"}, nil
}

func press(m Model, code rune) Model {
	return update(m, tea.KeyPressMsg{Code: code})
}

func typeText(m Model, text string) Model {
	return update(m, tea.KeyPressMsg{Code: []rune(text)[0], Text: text})
}

func TestAlternateScreenResizeAndFocusedScrolling(t *testing.T) {
	m := New(fakeGateway{}, "master-a", true)
	if !m.View().AltScreen || New(fakeGateway{}, "master-a", false).View().AltScreen {
		t.Fatal("alternate screen default/opt-out was not applied to the view")
	}
	m = update(m, tea.WindowSizeMsg{Width: 36, Height: 8})
	if m.width != 36 || m.height != 8 {
		t.Fatal("terminal dimensions were not retained")
	}
	m = update(m, keysLoaded{"master-a", 0, []string{"a", "b", "c", "d"}, time.Now(), nil})
	for i := 0; i < 3; i++ {
		m = press(m, tea.KeyDown)
	}
	if m.selected != "d" || m.offset != 3 {
		t.Fatalf("selected row should scroll into the narrow viewport: %q at %d", m.selected, m.offset)
	}
	m = press(m, tea.KeyTab)
	m = press(m, tea.KeyDown)
	if m.overviewOffset != 1 || m.selected != "d" {
		t.Fatal("focused overview should scroll without changing selection")
	}
	m = press(m, '?')
	m = press(m, tea.KeyDown)
	if !m.help || m.helpOffset != 1 || m.overviewOffset != 1 {
		t.Fatal("help scrolling should preserve underlying panel state")
	}
	m = press(m, tea.KeyEscape)
	if m.help || m.focus != 1 {
		t.Fatal("help should restore the original focus")
	}
	m = press(m, tea.KeyTab)
	m = press(m, tea.KeyDown)
	if m.detailOffset != 1 || m.selected != "d" {
		t.Fatal("details should scroll independently")
	}
	for i := 0; i < 40; i++ {
		m = press(m, tea.KeyDown)
	}
	if m.detailOffset != ui.ScrollLimit(m.viewData(), 2) {
		t.Fatal("scroll should stop at the last visible details row")
	}
	m = press(m, tea.KeyUp)
	if m.detailOffset != ui.ScrollLimit(m.viewData(), 2)-1 {
		t.Fatal("up should move immediately after reaching the end")
	}
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 25})
	if m.detailOffset > ui.ScrollLimit(m.viewData(), 2) || m.selected != "d" {
		t.Fatal("resize should clamp scrolling without resetting selection")
	}
	m = press(m, tea.KeyTab)
	m = press(m, tea.KeyUp)
	if m.selected != "c" || m.detailOffset != 0 {
		t.Fatal("changing selection should reset details scroll")
	}
}
func (fakeGateway) ReadPresence(context.Context) ([]string, error) {
	return nil, errors.New("presence denied")
}
func (fakeGateway) ReadSelectedGrains(context.Context, string) (fleet.Grains, error) {
	return fleet.Grains{"os": "Ubuntu"}, nil
}

func (fakeGateway) ReadStateTop(context.Context, []string) (assignments.Top, error) {
	return assignments.Top{"web-01": {"base": {"baseline", "apps.web"}}}, nil
}

func update(m Model, msg any) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestIndependentResultsAndStaleRequest(t *testing.T) {
	m := New(fakeGateway{}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 90, Height: 25})
	at := time.Now()
	m = update(m, keysLoaded{"master-a", 0, []string{"web-02", "web-01"}, at, nil})
	m = update(m, presenceLoaded{"master-a", 0, nil, at, errors.New("presence denied")})
	if m.selected != "web-01" || len(m.keys.Value) != 2 || m.presence.Err == nil {
		t.Fatalf("partial read lost inventory: %+v", m)
	}
	if !strings.Contains(m.View().Content, "unknown") {
		t.Fatal("missing unknown availability after failed presence")
	}
	m.search.SetValue("web-02")
	m.alignSelection()
	m.move(1)
	if m.selected != "web-02" {
		t.Fatalf("selection changed unexpectedly: %s", m.selected)
	}
	cmd := m.refresh()
	if cmd == nil {
		t.Fatal("refresh not started")
	}
	m = update(m, keysLoaded{"master-a", 0, []string{"old"}, at, nil})
	if m.selected != "web-02" || m.search.Value() != "web-02" {
		t.Fatal("stale result reset selection/filter")
	}
	m = update(m, keysLoaded{"master-a", 1, []string{"web-02", "db-01"}, at, nil})
	if m.selected != "web-02" || m.search.Value() != "web-02" {
		t.Fatal("refresh reset selection/filter")
	}
	m = update(m, presenceLoaded{"master-a", 1, []string{"web-02", "stranger"}, at, nil})
	if !strings.Contains(m.View().Content, "Connected: 1") || strings.Contains(m.View().Content, "stranger ·") {
		t.Fatal("presence ID outside accepted inventory counted or displayed")
	}
}

func TestGrainsResultBelongsToRequestedID(t *testing.T) {
	m := New(fakeGateway{}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 90, Height: 25})
	at := time.Now()
	m = update(m, keysLoaded{"master-a", 0, []string{"web-01", "web-02"}, at, nil})
	if m.inspect() == nil {
		t.Fatal("grains inspection did not start")
	}
	m.move(1) // focus moved for the test; a late return must remain with web-01
	m = update(m, grainsLoaded{"master-a", 1, "web-01", fleet.Grains{"os": "Ubuntu", "roles": []any{"web"}}, at, nil})
	if m.details["web-01"].observation.Value["os"] != "Ubuntu" || strings.Contains(m.View().Content, "os: Ubuntu") {
		t.Fatal("grains response displayed for the wrong minion")
	}
	m = update(m, grainsLoaded{"other-master", 1, "web-02", fleet.Grains{"os": "Other"}, at, nil})
	if _, ok := m.details["web-02"]; ok {
		t.Fatal("other context populated details")
	}
}

func TestCustomGrainsRemainWithMinionAndStaleOnFailure(t *testing.T) {
	m := New(fakeGateway{}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	at := time.Now()
	m = update(m, keysLoaded{"master-a", 0, []string{"web-01"}, at, nil})
	if m.inspect() == nil {
		t.Fatal("inspection did not start")
	}
	grains := fleet.Grains{"os": "Ubuntu", "site": map[string]any{"rack": 13}}
	m = update(m, grainsLoaded{"master-a", 1, "web-01", grains, at, nil})
	if view := m.View().Content; !strings.Contains(view, "site: {") || !strings.Contains(view, "rack") {
		t.Fatalf("custom nested grains missing from Details:\n%s", view)
	}
	if m.inspect() == nil {
		t.Fatal("repeat inspection did not start")
	}
	m = update(m, grainsLoaded{"master-a", 1, "web-01", fleet.Grains{"site": "wrong"}, at, nil})
	if m.details["web-01"].observation.Value["site"].(map[string]any)["rack"] != 13 {
		t.Fatal("superseded grain response replaced snapshot")
	}
	m = update(m, grainsLoaded{"master-a", 2, "web-01", nil, at.Add(time.Minute), errors.New("minion did not respond")})
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "site: {") || !strings.Contains(view, "stale/") || !m.details["web-01"].observation.At.Equal(at) {
		t.Fatalf("failed refresh lost stale details or observation time:\n%s", view)
	}
}

func TestSearchFollowsFocusedPanelWithoutTouchingOtherQuery(t *testing.T) {
	m := New(fakeGateway{}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 12})
	m = update(m, keysLoaded{"master-a", 0, []string{"node-a", "node-b"}, time.Now(), nil})
	m = update(m, presenceLoaded{"master-a", 0, []string{"node-a", "node-b"}, time.Now(), nil})
	m = press(m, '/')
	m = typeText(m, "b")
	if !m.search.Focused() || m.search.Value() != "b" || m.selected != "node-b" {
		t.Fatalf("target search did not filter IDs: selected %q query %q", m.selected, m.search.Value())
	}
	m = press(m, tea.KeyEscape)
	m = press(m, tea.KeyTab)
	m = press(m, '/') // Overview has no search and must not steal focus.
	if m.focus != 1 || m.search.Focused() || m.detailSearch.Focused() {
		t.Fatal("overview search changed the active panel")
	}
	m = press(m, tea.KeyTab)
	m = press(m, '/')
	m = typeText(m, "r")
	if !m.detailSearch.Focused() || m.detailSearch.Value() != "r" || m.search.Value() != "b" || m.selected != "node-b" {
		t.Fatal("details search changed target query, focus or selection")
	}
	if m.request != 0 {
		t.Fatal("search text was routed as a Fleet refresh")
	}
	m = press(m, tea.KeyEscape)
	m = press(m, tea.KeyTab)
	m = press(m, tea.KeyTab)
	m = press(m, tea.KeyTab)
	if m.detailSearch.Value() != "r" || m.search.Value() != "b" {
		t.Fatal("switching panels discarded either search")
	}
	m = press(m, '?')
	m = press(m, tea.KeyEscape)
	if m.detailSearch.Value() != "r" || m.focus != 2 {
		t.Fatal("help should return to Details with its search preserved")
	}
	m = press(m, '/')
	m = press(m, tea.KeyBackspace)
	if m.detailSearch.Value() != "" || m.search.Value() != "b" {
		t.Fatal("clearing Details search affected Targets")
	}
	m = press(m, tea.KeyEscape)
	m = press(m, tea.KeyTab)
	m.search.SetValue("")
	m.alignSelection()
	m.detailSearch.SetValue("previous grain")
	m.move(-1)
	if m.selected != "node-a" || m.detailSearch.Value() != "" {
		t.Fatal("selection did not update after clearing ID search")
	}
}

func TestPageKeysUseFocusedViewportAndClamp(t *testing.T) {
	m := New(fakeGateway{}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 36, Height: 9})
	ids := make([]string, 30)
	for i := range ids {
		ids[i] = "node-" + string(rune('a'+i))
	}
	m = update(m, keysLoaded{"master-a", 0, ids, time.Now(), nil})
	m = press(m, tea.KeyPgDown)
	if m.selected != ids[ui.ListRows(m.height)] || m.offset != ui.ListRows(m.height)-1 {
		t.Fatalf("target page down moved to %q at offset %d", m.selected, m.offset)
	}
	m = press(m, tea.KeyPgUp)
	if m.selected != ids[0] || m.offset != 0 {
		t.Fatal("target page up failed to restore first selected row")
	}
	m = press(m, tea.KeyTab)
	m = press(m, tea.KeyPgDown)
	if m.overviewOffset != min(ui.PageRows(m.height, 1, true), ui.ScrollLimit(m.viewData(), 1)) || m.selected != ids[0] {
		t.Fatal("overview paging changed selection or moved by wrong page")
	}
	m = press(m, tea.KeyTab)
	m = press(m, tea.KeyPgDown)
	if m.detailOffset != min(ui.PageRows(m.height, 2, true), ui.ScrollLimit(m.viewData(), 2)) {
		t.Fatal("details paging did not use its reduced content viewport")
	}
	m = press(m, '?')
	m = press(m, tea.KeyPgDown)
	if m.helpOffset != min(ui.PageRows(m.height, 3, true), ui.ScrollLimit(m.viewData(), 3)) {
		t.Fatal("help paging failed")
	}
	m = press(m, tea.KeyPgUp)
	if m.helpOffset != 0 || m.detailOffset == 0 {
		t.Fatal("help paging disturbed Details or failed to clamp")
	}
	m = press(m, tea.KeyEscape)
	m = press(m, tea.KeyPgUp)
	if m.detailOffset != 0 {
		t.Fatal("details page up failed to reach first row")
	}
}

func TestDetailFilterReclampsOnEditResizeAndRefresh(t *testing.T) {
	m := New(fakeGateway{}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 9})
	m = update(m, keysLoaded{"master-a", 0, []string{"node"}, time.Now(), nil})
	m.details["node"] = detail{observation: fleet.Observation[fleet.Grains]{Value: fleet.Grains{
		"os": "Ubuntu", "site": map[string]any{"rack": "A7"}, "role": "web",
	}, At: time.Now()}}
	m.focus = 2
	m.detailOffset = ui.ScrollLimit(m.viewData(), 2)
	m = press(m, '/')
	m = typeText(m, "rack")
	if m.detailOffset != 0 || m.detailSearch.Value() != "rack" || !m.detailSearch.Focused() {
		t.Fatal("editing the Details query did not reset scroll to the filtered result")
	}
	m = press(m, tea.KeyPgDown)
	if m.detailSearch.Value() != "rack" || m.focus != 2 || m.detailOffset != 0 {
		t.Fatal("page key while editing search moved the panel")
	}
	m = press(m, tea.KeyEscape)
	m = press(m, tea.KeyPgDown)
	m = update(m, tea.WindowSizeMsg{Width: 36, Height: 20})
	if m.detailOffset > ui.ScrollLimit(m.viewData(), 2) || m.detailSearch.Value() != "rack" {
		t.Fatal("resize did not clamp Details without discarding search")
	}
	m.details["node"] = detail{request: 1, observation: fleet.Observation[fleet.Grains]{Value: m.details["node"].observation.Value, At: time.Now()}}
	m = update(m, grainsLoaded{"master-a", 1, "node", fleet.Grains{"os": "Debian"}, time.Now(), nil})
	if m.detailOffset > ui.ScrollLimit(m.viewData(), 2) || m.detailSearch.Value() != "rack" {
		t.Fatal("updated grains did not reclamp the active filtered detail view")
	}
	m.detailOffset = ui.ScrollLimit(m.viewData(), 2)
	if !strings.Contains(m.View().Content, "No grains match") {
		t.Fatal("updated grains did not show the filtered no-match state")
	}
}
