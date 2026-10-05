package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"saltrtui/internal/keys"
)

type fakeKeys struct{ calls int }

func (f *fakeKeys) ListKeys(context.Context) ([]keys.Key, error) {
	return []keys.Key{{ID: "new-1", State: keys.Pending}, {ID: "web-01", State: keys.Accepted}}, nil
}
func (f *fakeKeys) ReadKey(context.Context, keys.Key) (keys.Detail, error) {
	return keys.Detail{Fingerprint: "aa:bb"}, nil
}
func (f *fakeKeys) ChangeKey(context.Context, keys.Key, keys.Action) error { f.calls++; return nil }

func TestKeyManagementConfirmationAndResult(t *testing.T) {
	backend := &fakeKeys{}
	m := NewWithKeys(fakeGateway{}, nil, nil, nil, nil, backend, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 90, Height: 18})
	m = press(m, '6')
	if m.activeView != 6 || !m.key.list.Busy {
		t.Fatal("keys did not load")
	}
	m = update(m, keyListLoaded{"master-a", 1, []keys.Key{{ID: "new-1", State: keys.Pending}, {ID: "web-01", State: keys.Accepted}}, time.Now(), nil})
	m = press(m, tea.KeyEnter)
	if m.key.focus != 1 || !m.key.details[m.key.selected].value.Busy {
		t.Fatal("details not requested")
	}
	m = update(m, keyDetailLoaded{"master-a", 1, m.key.selected, keys.Detail{Fingerprint: "aa:bb"}, time.Now(), nil})
	if !strings.Contains(ansi.Strip(m.View().Content), "Announced: Unknown") {
		t.Fatal("pending timestamp mislabeled")
	}
	m = press(m, 'a')
	if m.key.confirm != keys.Accept || !strings.Contains(ansi.Strip(m.View().Content), "Confirm accept") {
		t.Fatal("confirmation missing")
	}
	m = press(m, tea.KeyEscape)
	if m.key.confirm != "" || backend.calls != 0 {
		t.Fatal("escape did not cancel")
	}
	m = press(m, 'a')
	model, cmd := m.Update(tea.KeyPressMsg{Code: 'y'})
	m = model.(Model)
	if cmd == nil || !m.key.busy || backend.calls != 0 {
		t.Fatal("action dispatched before async confirmation")
	}
	msg := cmd()
	if backend.calls != 1 {
		t.Fatal("confirmed key action not dispatched")
	}
	m = update(m, msg)
	if !m.key.list.Busy {
		t.Fatal("post-action inventory was not refreshed")
	}
	m = update(m, keyListLoaded{"master-a", 2, []keys.Key{{ID: "new-1", State: keys.Accepted}}, time.Now(), nil})
	if m.key.selected.State != keys.Accepted {
		t.Fatal("new key state not reflected")
	}
}

func TestKeySearchLateResultsAndFailure(t *testing.T) {
	m := NewWithKeys(fakeGateway{}, nil, nil, nil, nil, &fakeKeys{}, "master-a", false)
	m = update(m, tea.WindowSizeMsg{Width: 35, Height: 10})
	m = press(m, '6')
	at := time.Now()
	m = update(m, keyListLoaded{"master-a", 1, []keys.Key{{ID: "new-1", State: keys.Pending}, {ID: "web-01", State: keys.Accepted}}, at, nil})
	m = press(m, '/')
	m = typeText(m, "web")
	if m.key.selected.ID != "web-01" {
		t.Fatal("search did not filter")
	}
	m = press(m, tea.KeyEnter)
	m = update(m, keyDetailLoaded{"master-a", 1, keys.Key{ID: "new-1", State: keys.Pending}, keys.Detail{Fingerprint: "wrong"}, time.Now(), nil})
	if strings.Contains(ansi.Strip(m.View().Content), "wrong") {
		t.Fatal("late detail leaked to selection")
	}
	m = update(m, keyListLoaded{"other", 1, nil, time.Now(), nil})
	if m.key.selected.ID != "web-01" {
		t.Fatal("wrong context replaced selection")
	}
	m = update(m, keyListLoaded{"master-a", 0, nil, time.Now(), nil})
	if m.key.selected.ID != "web-01" {
		t.Fatal("superseded inventory replaced selection")
	}
}

func TestDuplicateKeyStateDisablesAction(t *testing.T) {
	m := NewWithKeys(fakeGateway{}, nil, nil, nil, nil, &fakeKeys{}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 110, Height: 25})
	m = press(m, '6')
	m = update(m, keyListLoaded{"master-a", 1, []keys.Key{{ID: "same", State: keys.Accepted}, {ID: "same", State: keys.Denied}}, time.Now(), nil})
	if !m.ambiguousKey() {
		t.Fatal("duplicate ID in different Salt states not recognized")
	}
	m = press(m, 'd')
	m = press(m, tea.KeyTab)
	if m.key.confirm != "" || !m.keysViewData().Ambiguous || !strings.Contains(ansi.Strip(m.View().Content), "This ID occurs") {
		t.Fatal("ambiguous key allowed revoke or hid warning")
	}
}
