package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"saltrtui/internal/assignments"
)

func TestAssignmentsTopLevelTreeAndFleetContext(t *testing.T) {
	m := New(fakeGateway{}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 24})
	m = update(m, keysLoaded{"master-a", 0, []string{"web-02", "web-01"}, time.Now(), nil})
	m.search.SetValue("web-01")
	m.alignSelection()
	m = press(m, '3')
	if m.activeView != 9 || m.assignments == nil || !m.assignments.observation.Busy {
		t.Fatal("Assignments tab did not open and start its first fetch")
	}
	at := time.Now()
	m = update(m, stateTopLoaded{"master-a", m.assignments.request, []string{"web-01", "web-02"}, assignments.Top{
		"web-01": {"base": {"baseline", "apps.web"}},
		"web-02": {"base": {"baseline"}},
	}, at, nil})
	if got := m.assignments.selected; got != assignments.EnvironmentID("base") {
		t.Fatalf("initial tree selection = %q", got)
	}
	if view := m.View().Content; !strings.Contains(view, "3 Assignments") || !strings.Contains(view, "apps.web") || strings.Contains(view, "web-01") {
		t.Fatalf("unexpected collapsed tree:\n%s", view)
	}
	m = press(m, tea.KeyDown)
	m = press(m, tea.KeyDown)
	m = press(m, tea.KeyEnter)
	if view := m.View().Content; !strings.Contains(view, "web-01") || !strings.Contains(view, "web-02") {
		t.Fatalf("expanding baseline did not reveal its responding minions:\n%s", view)
	}
	m = press(m, '1')
	if m.activeView != 0 || m.selected != "web-01" || m.search.Value() != "web-01" {
		t.Fatal("switching back from Assignments lost Fleet context")
	}
}

func TestAssignmentsFilterAndFailurePreserveTree(t *testing.T) {
	m := New(fakeGateway{}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 90, Height: 18})
	m = update(m, keysLoaded{"master-a", 0, []string{"web-01"}, time.Now(), nil})
	m = press(m, '3')
	at := time.Now()
	m = update(m, stateTopLoaded{"master-a", 1, []string{"web-01"}, assignments.Top{"web-01": {"base": {"baseline", "apps.web"}}}, at, nil})
	m = press(m, '/')
	m = typeText(m, "web-01")
	if nodes := m.visibleAssignmentNodes(); len(nodes) != 5 || nodes[2].Minion != "web-01" || nodes[4].Minion != "web-01" {
		t.Fatalf("filter did not retain the matching tree ancestry: %#v", nodes)
	}
	m = press(m, tea.KeyEscape)
	if m.assignments.filter.Value() != "" || m.assignments.filter.Focused() {
		t.Fatal("escape did not clear the local assignment filter")
	}
	_ = m.loadAssignments()
	m = update(m, stateTopLoaded{"master-a", 2, []string{"web-01"}, nil, at.Add(time.Minute), errors.New("minion did not respond")})
	if len(m.assignments.observation.Value) != 1 || m.assignments.observation.Err == nil || !m.assignments.observation.At.Equal(at) {
		t.Fatal("failed refresh discarded the prior assignment tree")
	}
}
