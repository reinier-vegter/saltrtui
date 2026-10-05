package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestTargetPreviewSuggestionExclusionAndSave(t *testing.T) {
	m := New(fakeGateway{}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 68, Height: 12})
	m = update(m, keysLoaded{"master-a", 0, []string{"web-01", "web-02", "db-01"}, time.Now(), nil})
	m = press(m, 't')
	if m.activeView != 3 || m.target.input.Value() != "db-01" {
		t.Fatal("builder should start from selected ID")
	}
	m.target.input.SetValue("we")
	m.targetEdit()
	m = press(m, tea.KeyDown)
	m = press(m, tea.KeyTab)
	if m.target.input.Value() != "web-02" || m.search.Value() != "" {
		t.Fatal("completion did not insert selected ID independently of Fleet search")
	}
	m = ctrl(m, 't')
	m.target.input.SetValue("web-*")
	m.targetEdit()
	m = ctrl(m, 'p')
	if !m.target.busy || m.target.request != 1 {
		t.Fatal("preview did not initiate fresh key read")
	}
	at := time.Now()
	m = update(m, targetLoaded{"master-a", 1, m.target.revision, []string{"web-01", "web-02", "db-01"}, at, nil})
	if len(m.target.preview.Value.IDs) != 2 || m.target.stale || m.target.preview.At.IsZero() {
		t.Fatalf("fresh candidate preview missing: %+v", m.target.preview)
	}
	m = ctrl(m, 'n')
	if !m.target.resultsFocus || m.target.offset == 0 {
		t.Fatal("result focus should reveal selected match in a short viewport")
	}
	m = press(m, 'x')
	if m.target.mode != "list" || m.target.input.Value() != "web-02" || !m.target.stale {
		t.Fatal("removal must become a stale explicit list")
	}
	m = ctrl(m, 's')
	if m.savedTarget != nil {
		t.Fatal("stale exclusion was saved")
	}
	m = ctrl(m, 'p')
	m = update(m, targetLoaded{"master-a", 2, m.target.revision, []string{"web-01", "web-02"}, at.Add(time.Second), nil})
	m = ctrl(m, 's')
	if m.savedTarget == nil || len(m.savedTarget.preview.IDs) != 1 || m.savedTarget.preview.IDs[0] != "web-02" {
		t.Fatalf("effective target not saved: %+v", m.savedTarget)
	}
	m = press(m, tea.KeyEscape)
	if m.activeView != 0 || m.selected != "db-01" || m.savedTarget == nil {
		t.Fatal("closing builder lost Fleet position or saved target")
	}
}

func TestTargetLateAndFailedPreviewCannotBecomeCurrent(t *testing.T) {
	m := New(fakeGateway{}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 26})
	m = update(m, keysLoaded{"master-a", 0, []string{"web-01"}, time.Now(), nil})
	m = press(m, 't')
	m = ctrl(m, 'p')
	m = typeText(m, "x")
	at := time.Now()
	m = update(m, targetLoaded{"master-a", 1, 0, []string{"web-01"}, at, nil})
	if !m.target.stale || !m.target.preview.At.IsZero() {
		t.Fatal("response for edited draft must not become current")
	}
	m = ctrl(m, 'p')
	m = update(m, targetLoaded{"other-master", 2, m.target.revision, []string{"web-01"}, at, nil})
	if !m.target.busy {
		t.Fatal("other master cleared pending request")
	}
	m = update(m, targetLoaded{"master-a", 2, m.target.revision, nil, at, errors.New("keys denied")})
	if m.target.preview.Err == nil || !strings.Contains(m.View().Content, "Preview failed") {
		t.Fatal("failure must be visible and block saving")
	}
	m = ctrl(m, 's')
	if m.savedTarget != nil {
		t.Fatal("failed preview saved a target")
	}
}

func TestReopenBuilderStartsFromSelectionAndDropsOldRequest(t *testing.T) {
	m := New(fakeGateway{}, "master-a", true)
	m = update(m, keysLoaded{"master-a", 0, []string{"db-01", "web-01"}, time.Now(), nil})
	m = press(m, 't')
	m = ctrl(m, 'p')
	m = press(m, tea.KeyEscape)
	m = press(m, tea.KeyDown)
	m = press(m, 't')
	if m.target.input.Value() != "web-01" {
		t.Fatal("builder did not start from current selection")
	}
	m = ctrl(m, 'p')
	at := time.Now()
	m = update(m, targetLoaded{"master-a", 1, 0, []string{"db-01"}, at, nil})
	if !m.target.busy || !m.target.preview.At.IsZero() {
		t.Fatal("old builder request overwrote new preview")
	}
	m = update(m, targetLoaded{"master-a", 2, 0, []string{"web-01"}, at, nil})
	if m.target.busy || len(m.target.preview.Value.IDs) != 1 || m.target.preview.Value.IDs[0] != "web-01" {
		t.Fatal("new builder preview did not resolve")
	}
}

func ctrl(m Model, code rune) Model {
	return update(m, tea.KeyPressMsg{Code: code, Mod: tea.ModCtrl})
}
