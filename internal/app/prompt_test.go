package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"saltrtui/internal/keys"
	"saltrtui/internal/release"
	"saltrtui/internal/ui"
)

func keyPromptModel(backend *fakeKeys, state keys.State, key rune) Model {
	m := NewWithKeys(fakeGateway{}, nil, nil, nil, nil, backend, "master-a", true)
	m.width, m.height, m.activeView = 100, 25, 6
	m = update(m, keyListLoaded{"master-a", 0, []keys.Key{{ID: "pinned-01", State: state}}, time.Now(), nil})
	return press(m, key)
}

func TestAllKeyDecisionsFitBeforeDispatchAndCancelRestoresFocus(t *testing.T) {
	for _, action := range []struct {
		key   rune
		state keys.State
	}{{'A', keys.Pending}, {'b', keys.Pending}, {'d', keys.Accepted}} {
		backend := &fakeKeys{}
		m := keyPromptModel(backend, action.state, action.key)
		defer m.Close()
		if m.key.confirm == "" || m.key.confirmFocus != 0 {
			t.Fatal("prompt failed to pin invoking focus")
		}
		m = update(m, tea.WindowSizeMsg{Width: 20, Height: 3})
		model, cmd := m.Update(tea.KeyPressMsg{Code: 'y'})
		m = model.(Model)
		if cmd != nil || m.key.confirm == "" || m.key.busy || backend.calls != 0 {
			t.Fatal("undersized key prompt consumed approval or sent action")
		}
		m = press(m, tea.KeyEscape)
		if m.key.confirm != "" || m.key.focus != 0 || backend.calls != 0 {
			t.Fatal("cancel did not restore invoking focus")
		}
		m = update(m, tea.WindowSizeMsg{Width: 100, Height: 25})
		m = press(m, action.key)
		model, cmd = m.Update(tea.KeyPressMsg{Code: 'y'})
		m = model.(Model)
		if cmd == nil || !m.key.busy || m.key.confirm != "" {
			t.Fatal("fitting approval did not dispatch asynchronously")
		}
		m = press(m, 'y')
		cmd()
		if backend.calls != 1 || backend.actedOn[0].ID != "pinned-01" {
			t.Fatal("wrong target or duplicate dispatch")
		}
	}
}

func TestKeyApprovalInvalidationAndPinnedScope(t *testing.T) {
	for _, name := range []string{"missing", "changed", "ambiguous", "error", "selection", "refresh", "context"} {
		t.Run(name, func(t *testing.T) {
			backend := &fakeKeys{}
			m := keyPromptModel(backend, keys.Pending, 'A')
			defer m.Close()
			switch name {
			case "missing":
				m = update(m, keyListLoaded{"master-a", 0, nil, time.Now(), nil})
			case "changed":
				m = update(m, keyListLoaded{"master-a", 0, []keys.Key{{ID: "pinned-01", State: keys.Accepted}}, time.Now(), nil})
			case "ambiguous":
				m = update(m, keyListLoaded{"master-a", 0, []keys.Key{{ID: "pinned-01", State: keys.Pending}, {ID: "pinned-01", State: keys.Denied}}, time.Now(), nil})
			case "error":
				m = update(m, keyListLoaded{"master-a", 0, nil, time.Now(), errors.New("unavailable")})
			case "selection":
				m.key.selected = keys.Key{ID: "other", State: keys.Pending}
				view := ansi.Strip(strings.Join(mustKeyPromptRows(m), "\n"))
				if !strings.Contains(view, "pinned-01") || strings.Contains(view, "other") {
					t.Fatal("prompt retargeted to mutable selection")
				}
			case "refresh":
				m.loadKeys()
			case "context":
				m.context = "other-master"
			}
			model, cmd := m.Update(tea.KeyPressMsg{Code: 'y'})
			m = model.(Model)
			if cmd != nil || m.key.confirm != "" || backend.calls != 0 || !strings.Contains(m.key.status, "canceled") {
				t.Fatalf("invalid approval not explained: %s", m.key.status)
			}
		})
	}
}

func mustKeyPromptRows(m Model) []string {
	// Read the complete view: the pinned facts are confined to the bottom box.
	view := m.View().Content
	rows := strings.Split(view, "\n")
	for i, row := range rows {
		if strings.Contains(ansi.Strip(row), "Confirm accept") {
			return rows[i:]
		}
	}
	return nil
}

func readyHighstatePrompt(t *testing.T, backend *fakeHighstate) Model {
	t.Helper()
	m := highstateModel(backend)
	m = press(m, 'h')
	model, cmd := m.Update(tea.KeyPressMsg{Code: 'p'})
	return update(model.(Model), cmd())
}

func TestHighstateApprovalExpiryAndInvalidation(t *testing.T) {
	for _, reason := range []string{"expiry", "inventory error", "removed", "context"} {
		backend := &fakeHighstate{}
		m := readyHighstatePrompt(t, backend)
		defer m.Close()
		m = press(m, 'a')
		switch reason {
		case "expiry":
			m.highstate.preview.At = time.Now().Add(-highstatePreviewLifetime - time.Second)
			m = update(m, highstateApprovalExpired{m.context, m.highstate.id, m.highstate.request, m.highstate.confirmation, m.highstate.preview.At})
		case "inventory error":
			m = update(m, keysLoaded{m.context, m.request, nil, time.Now(), errors.New("inventory unavailable")})
		case "removed":
			m = update(m, keysLoaded{m.context, m.request, []string{"web-02"}, time.Now(), nil})
		case "context":
			m.context = "another-master"
		}
		model, cmd := m.Update(tea.KeyPressMsg{Code: 'y'})
		m = model.(Model)
		if cmd != nil || m.highstate.confirm || backend.apply != 0 || !strings.Contains(m.highstate.status, "canceled") {
			t.Fatalf("lost eligibility not explained (%s): %s", reason, m.highstate.status)
		}
	}
}

func TestHighstateStaleExpiryAndScrollRestoration(t *testing.T) {
	m := readyHighstatePrompt(t, &fakeHighstate{})
	defer m.Close()
	m.height = 12
	m.highstate.offset = 1
	m = press(m, 'a')
	old := highstateApprovalExpired{m.context, m.highstate.id, m.highstate.request, m.highstate.confirmation, m.highstate.preview.At}
	m = press(m, 'n')
	if m.highstate.offset != 1 {
		t.Fatal("cancel lost invoking report offset")
	}
	m = press(m, 'a')
	m = update(m, old)
	if !m.highstate.confirm {
		t.Fatal("old expiry canceled newer approval")
	}
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 100})
	m = press(m, 'n')
	if m.highstate.offset != 0 {
		t.Fatal("restored invalid offset after resize")
	}
}

func TestPromptPasteCannotReachBackgroundInputs(t *testing.T) {
	for _, view := range []int{6, 7, 8} {
		m := keyPromptModel(&fakeKeys{}, keys.Pending, 'A')
		defer m.Close()
		m.activeView = view
		m.search.Focus()
		m.detailSearch.Focus()
		m.key.search.Focus()
		m.search.SetValue("fleet")
		m.detailSearch.SetValue("details")
		m.key.search.SetValue("keys")
		m = update(m, tea.PasteMsg{Content: "should-not-appear"})
		if m.search.Value() != "fleet" || m.detailSearch.Value() != "details" || m.key.search.Value() != "keys" {
			t.Fatalf("view %d paste escaped modal ownership", view)
		}
	}
}

func TestUpdateCancelBeforeFitAndLongDestinationRecheck(t *testing.T) {
	for _, size := range [][2]int{{59, 11}, {1, 1}, {100, 25}} {
		m := New(fakeGateway{}, "master-a", true)
		defer m.Close()
		m.width, m.height, m.activeView, m.updatePhase = size[0], size[1], 8, "confirm"
		m.updateIndex = 1
		model, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = model.(Model)
		if cmd != nil || m.activeView != 0 {
			t.Fatal("Cancel blocked by fit or dispatched")
		}
	}
	m := New(fakeGateway{}, "master-a", true)
	defer m.Close()
	m.width, m.height, m.activeView, m.updatePhase = 100, 25, 8, "confirm"
	m.installation = release.Installation{Current: "/opt/" + strings.Repeat("long-dir/", 100) + "saltrtui", Writable: true}
	model, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = model.(Model)
	if cmd != nil || m.updatePhase != "confirm" || !strings.Contains(ansi.Strip(m.View().Content), "resize") {
		t.Fatal("long destination bypassed measured fit")
	}
}

func TestUpdateAffirmativeMeasuredBoundary(t *testing.T) {
	for _, width := range []int{60, 64, 80} {
		for _, writable := range []bool{true, false} {
			v := ui.UpdateViewData{Width: width, Phase: "confirm", Running: "v1.0.0", Available: "v1.0.1", Current: "/usr/local/bin/saltrtui", Writable: writable}
			minimum := 0
			for height := 1; height <= 40; height++ {
				v.Height = height
				if ui.UpdatePromptLayout(v).FitsConfirmation {
					minimum = height
					break
				}
			}
			if minimum == 0 {
				t.Fatal("update never becomes confirmable")
			}
			for _, height := range []int{minimum - 1, minimum} {
				m := New(fakeGateway{}, "master-a", true)
				m.width, m.height, m.activeView, m.updatePhase = width, height, 8, "confirm"
				m.version, m.availableUpdate = v.Running, v.Available
				m.installation = release.Installation{Current: v.Current, Writable: writable}
				model, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				m = model.(Model)
				if (cmd != nil) != (height == minimum) || (m.updatePhase == "downloading") != (height == minimum) {
					t.Fatalf("boundary %dx%d writable=%v minimum=%d", width, height, writable, minimum)
				}
				m.Close() // Do not execute the returned download command.
			}
			t.Logf("width=%d writable=%v: smallest fitting height=%d", width, writable, minimum)
		}
	}
}

func TestHighstateExpiryCommandCleanup(t *testing.T) {
	for _, action := range []string{"cancel", "apply", "invalidation", "close", "new preview"} {
		m := readyHighstatePrompt(t, &fakeHighstate{})
		model, expiry := m.Update(tea.KeyPressMsg{Code: 'a'})
		m = model.(Model)
		if expiry == nil {
			t.Fatal("confirmation did not schedule expiry")
		}
		switch action {
		case "cancel":
			m = press(m, 'n')
		case "apply":
			m = press(m, 'y')
		case "invalidation":
			m = update(m, keysLoaded{m.context, m.request, nil, time.Now(), errors.New("unavailable")})
		case "close":
			m.Close()
		case "new preview":
			m.previewHighstate()
		}
		result := make(chan tea.Msg, 1)
		go func() { result <- expiry() }()
		select {
		case msg := <-result:
			if msg != nil {
				t.Fatalf("%s left expiry alive: %#v", action, msg)
			}
		case <-time.After(time.Second):
			m.Close()
			t.Fatalf("%s did not release expiry command", action)
		}
		m.Close()
	}
}

func TestHighstateExpiryCommandReportsExactScope(t *testing.T) {
	m := readyHighstatePrompt(t, &fakeHighstate{})
	defer m.Close()
	m.highstate.preview.At = time.Now().Add(-highstatePreviewLifetime + time.Second)
	model, cmd := m.Update(tea.KeyPressMsg{Code: 'a'})
	m = model.(Model)
	if cmd == nil {
		t.Fatal("expiry command missing")
	}
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	select {
	case msg := <-result:
		expired, ok := msg.(highstateApprovalExpired)
		if !ok || expired.context != m.context || expired.id != m.highstate.id || expired.request != m.highstate.request || expired.confirmation != m.highstate.confirmation || !expired.previewAt.Equal(m.highstate.preview.At) {
			t.Fatalf("unscoped expiry: %#v", msg)
		}
		m = update(m, msg)
		if m.highstate.confirm || !strings.Contains(m.highstate.status, "expired") {
			t.Fatal("expiry command failed to invalidate approval")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("expiry command did not deliver deadline")
	}
}

func TestKeySelectionVisibleAfterCancelAndResize(t *testing.T) {
	m := keyPromptModel(&fakeKeys{}, keys.Pending, 'A')
	defer m.Close()
	m = press(m, 'n')
	items := make([]keys.Key, 40)
	for i := range items {
		items[i] = keys.Key{ID: fmt.Sprintf("key-%02d", i), State: keys.Pending}
	}
	m.key.list.Value, m.key.selected = items, items[30]
	m.height, m.key.offset = 40, 10
	m = press(m, 'A')
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 12})
	m = press(m, 'n')
	if 30 < m.key.offset || 30 >= m.key.offset+ui.KeysListRows(m.keysViewData()) || m.key.focus != 0 {
		t.Fatal("cancel restored viewport excluding selection")
	}
}

func TestKeysViewportClampsAgainstDynamicActionsBox(t *testing.T) {
	m := keyPromptModel(&fakeKeys{}, keys.Pending, 'A')
	defer m.Close()
	m = press(m, 'n')
	items := make([]keys.Key, 40)
	for i := range items {
		items[i] = keys.Key{ID: fmt.Sprintf("key-%02d", i), State: keys.Pending}
	}
	m.width, m.height = 100, 24
	m.key.list.Value, m.key.selected, m.key.offset, m.key.status = items, items[30], 999, "A status reason is visible"
	model, _ := m.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	m = model.(Model)
	rows := ui.KeysListRows(m.keysViewData())
	if m.key.offset > max(0, len(items)-rows) || 30 < m.key.offset || 30 >= m.key.offset+rows {
		t.Fatalf("selection/offset not clamped to dynamic list capacity: offset=%d rows=%d", m.key.offset, rows)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "> key-30") {
		t.Fatal("selected key disappeared after the Actions box changed height")
	}
}

func TestKeysNavigationKeepsSelectionVisibleAtSixRows(t *testing.T) {
	m := keyPromptModel(&fakeKeys{}, keys.Pending, 'A')
	defer m.Close()
	m = press(m, 'n')
	items := make([]keys.Key, 40)
	for i := range items {
		items[i] = keys.Key{ID: fmt.Sprintf("key-%02d", i), State: keys.Pending}
	}
	m.width, m.height, m.key.focus = 100, 6, 0
	m.key.list.Value, m.key.selected, m.key.offset = items, items[0], 0
	m.moveKey(30)
	if !strings.Contains(ansi.Strip(m.View().Content), "> key-30 · pending") {
		t.Fatal("selected key not visible after navigation in a 6-row terminal")
	}
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 6})
	m = model.(Model)
	if !strings.Contains(ansi.Strip(m.View().Content), "> key-30 · pending") {
		t.Fatal("selected key not visible after resize in a 6-row terminal")
	}
}
