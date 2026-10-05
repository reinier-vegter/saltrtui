package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"saltrtui/internal/highstate"
)

type fakeHighstate struct {
	preview, apply int
	applyErr       error
}

func (f *fakeHighstate) Preview(context.Context, string) (highstate.Report, error) {
	f.preview++
	return highstate.Report{Steps: []highstate.Step{{ID: "pkg_|-baseline", Changes: json.RawMessage(`{"new":"yes"}`)}}, Proposed: 1}, nil
}

func (f *fakeHighstate) Apply(context.Context, string) (highstate.Report, error) {
	f.apply++
	if f.applyErr != nil {
		return highstate.Report{}, f.applyErr
	}
	result := true
	return highstate.Report{Steps: []highstate.Step{{ID: "pkg_|-baseline", Result: &result, Changes: json.RawMessage(`{"installed":"yes"}`)}}, Changed: 1}, nil
}

func highstateModel(backend *fakeHighstate) Model {
	m := NewWithHighstate(fakeGateway{}, nil, nil, nil, nil, nil, backend, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 20})
	m = update(m, keysLoaded{"master-a", 0, []string{"web-01", "web-02"}, time.Now(), nil})
	return m
}

func TestHighstateReviewConfirmationAndOneApply(t *testing.T) {
	backend := &fakeHighstate{}
	m := highstateModel(backend)
	m = press(m, 'h')
	if m.activeView != 7 || m.highstate.id != "web-01" || backend.preview != 0 {
		t.Fatal("Fleet highstate view must pin selected ID without dispatch")
	}
	m = press(m, 'a')
	if m.highstate.confirm || backend.apply != 0 {
		t.Fatal("Apply allowed before preview")
	}
	model, command := m.Update(tea.KeyPressMsg{Code: 'p'})
	m = model.(Model)
	if command == nil || !m.highstate.busy || backend.preview != 0 {
		t.Fatal("preview did not start asynchronously")
	}
	m = press(m, tea.KeyEscape)
	if m.activeView != 7 {
		t.Fatal("left while preview was in flight")
	}
	m = update(m, command())
	if backend.preview != 1 || !m.canApplyHighstate() || !strings.Contains(ansi.Strip(m.View().Content), "proposed") {
		t.Fatal("preview result did not become reviewable")
	}
	m.highstate.preview.At = time.Now().Add(-highstatePreviewLifetime - time.Minute)
	m = press(m, 'a')
	if m.highstate.confirm || !m.highstateViewData().Expired {
		t.Fatal("expired preview still enabled Apply")
	}
	m.highstate.preview.At = time.Now()
	m = press(m, 'a')
	if !m.highstate.confirm || !strings.Contains(ansi.Strip(m.View().Content), "Confirm Apply") {
		t.Fatal("exact-target confirmation missing")
	}
	m = press(m, tea.KeyEscape)
	if m.highstate.confirm || backend.apply != 0 {
		t.Fatal("cancel dispatched state job")
	}
	m = press(m, 'a')
	model, command = m.Update(tea.KeyPressMsg{Code: 'y'})
	m = model.(Model)
	if command == nil || !m.highstate.busy || backend.apply != 0 {
		t.Fatal("apply dispatched before confirmed async command")
	}
	m = press(m, 'a')
	m = press(m, tea.KeyEscape)
	if m.activeView != 7 {
		t.Fatal("in-flight apply was abandoned")
	}
	m = update(m, command())
	if backend.apply != 1 || m.highstate.apply.Value.Changed != 1 || m.canApplyHighstate() {
		t.Fatal("apply did not return or allowed duplicate dispatch")
	}
	m = press(m, 'a')
	m = press(m, 'y')
	if backend.apply != 1 {
		t.Fatal("duplicate Apply without new preview")
	}
	m = press(m, tea.KeyEscape)
	if m.activeView != 0 || m.selected != "web-01" {
		t.Fatal("Fleet selection lost on return")
	}
	m.Close()
}

func TestHighstateStaleResultsSelectionAndUnknownOutcome(t *testing.T) {
	backend := &fakeHighstate{applyErr: errors.New("timeout")}
	m := highstateModel(backend)
	m = press(m, 'h')
	model, command := m.Update(tea.KeyPressMsg{Code: 'p'})
	m = model.(Model)
	m = update(m, highstateLoaded{context: "other", id: "web-01", request: m.highstate.request, preview: true, report: highstate.Report{Proposed: 1}, at: time.Now()})
	if !m.highstate.busy {
		t.Fatal("cross-context result cleared busy state")
	}
	m = update(m, command())
	m = update(m, keysLoaded{"master-a", 0, []string{"web-02"}, time.Now(), nil})
	if m.canApplyHighstate() {
		t.Fatal("selection change left preview authorized")
	}
	m = press(m, 'a')
	if m.highstate.confirm || backend.apply != 0 {
		t.Fatal("changed selection dispatched an Apply")
	}
	m = press(m, tea.KeyEscape)
	m = press(m, 'h')
	if m.highstate.id != "web-02" {
		t.Fatal("new review targeted old minion")
	}
	m = update(m, highstateLoaded{context: "master-a", id: "web-01", request: m.highstate.request - 1, preview: true, at: time.Now()})
	if !m.highstate.preview.At.IsZero() {
		t.Fatal("late preview leaked into new review")
	}
	m = press(m, 'p')
	m = update(m, highstateLoaded{context: "master-a", id: "web-02", request: m.highstate.request, preview: true,
		report: highstate.Report{Steps: []highstate.Step{{ID: "failed"}}, Failed: 1}, at: time.Now()})
	if m.highstate.preview.At.IsZero() {
		t.Fatal("expected preview fixture was ignored")
	}
	m = press(m, 'a')
	if m.highstate.confirm {
		t.Fatal("failed preview enabled Apply")
	}
	m = press(m, 'p')
	m = update(m, highstateLoaded{context: "master-a", id: "web-02", request: m.highstate.request, preview: true,
		report: highstate.Report{Steps: []highstate.Step{{ID: "ok"}}, Proposed: 1}, at: time.Now()})
	m = press(m, 'a')
	model, apply := m.Update(tea.KeyPressMsg{Code: 'y'})
	m = model.(Model)
	m = update(m, apply())
	if backend.apply != 1 || !strings.Contains(m.highstate.status, "outcome unknown") || m.canApplyHighstate() {
		t.Fatal("timed out apply was reported as success or automatically retryable")
	}
	m.Close()
}

func TestHighstateReviewBoundsSensitiveFields(t *testing.T) {
	result := true
	secret := strings.Repeat("s", 3000)
	report := highstate.Report{Steps: []highstate.Step{{ID: strings.Repeat("i", 300), Result: &result,
		Comment: secret, Changes: json.RawMessage(`{"diff":"` + secret + `"}`)}}}
	v := stateViewReport(report)
	if len(v.Steps) != 1 || len(v.Steps[0].ID) > 300 || strings.Contains(v.Steps[0].Comment, secret) || strings.Contains(v.Steps[0].Changes, secret) ||
		!strings.Contains(v.Steps[0].Comment, "(display truncated)") || !strings.Contains(v.Steps[0].Changes, "(display truncated)") {
		t.Fatalf("unbounded highstate review fields: %+v", v.Steps)
	}
}
