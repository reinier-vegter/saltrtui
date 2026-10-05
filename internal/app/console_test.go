package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"saltrtui/internal/console"
)

type fakeConsole struct {
	calls   []string
	account string
}

func (f *fakeConsole) Identity(context.Context, string) (console.Identity, error) {
	account := f.account
	if account == "" {
		account = "root"
	}
	return console.Identity{Account: account, Home: "/" + account, Cwd: "/srv"}, nil
}
func (f *fakeConsole) History(context.Context, string, string) ([]string, error) {
	return []string{"echo saved", "pwd"}, nil
}
func (f *fakeConsole) Run(_ context.Context, id, line, cwd string) (console.Result, error) {
	f.calls = append(f.calls, id+":"+cwd+":"+line)
	return console.Result{Stdout: "result", Retcode: 2}, nil
}
func (f *fakeConsole) ChangeDirectory(_ context.Context, id, cwd string) (string, error) {
	if cwd != "/srv/new" {
		return "", errors.New("unavailable")
	}
	return cwd, nil
}

func consoleKeyUpdate(m Model, key rune) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyPressMsg{Code: key})
	return next.(Model), cmd
}

func TestConsoleOpenHistoryRunCwdAndStaleResults(t *testing.T) {
	backend := &fakeConsole{}
	m := NewWithConsole(fakeGateway{}, fakeJobs{}, backend, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 80, Height: 16})
	m = update(m, keysLoaded{"master-a", 0, []string{"web-01", "web-02"}, time.Now(), nil})
	m, cmd := consoleKeyUpdate(m, 's')
	if cmd == nil || m.activeView != 2 || m.consoleID != "web-01" {
		t.Fatal("Fleet selection did not open its console")
	}
	m = update(m, cmd())
	state := m.consoles[m.consoleKey("web-01")]
	if !state.histBusy || state.identity.Account != "root" {
		t.Fatal("execution account/history pre-fetch missing")
	}
	// A command while history pre-fetch is pending must not invalidate its result.
	m = typeText(m, "echo session")
	m, cmd = consoleKeyUpdate(m, tea.KeyEnter)
	if cmd == nil || !state.busy {
		t.Fatal("command not dispatched")
	}
	m = update(m, consoleCommandLoaded{context: "other", id: "web-01", request: state.request, line: "wrong"})
	if !state.busy {
		t.Fatal("other context result was applied")
	}
	m = update(m, cmd())
	if state.busy || state.entries[0].result.Retcode != 2 || backend.calls[0] != "web-01:/srv:echo session" {
		t.Fatalf("command result not retained: %+v %v", state.entries, backend.calls)
	}
	m = update(m, consoleHistoryLoaded{context: "master-a", id: "web-01", request: state.setupRequest, items: []string{"echo saved", "pwd"}})
	if len(state.history.Saved) != 2 {
		t.Fatal("pre-fetch was discarded by command dispatch")
	}
	m = press(m, tea.KeyUp)
	if state.input.Value() != "echo session" {
		t.Fatal("session command missing from merged history")
	}
	m = press(m, tea.KeyUp)
	if state.input.Value() != "pwd" {
		t.Fatal("saved history missing from up-arrow navigation")
	}
	m = press(m, tea.KeyDown)
	m = press(m, tea.KeyDown)
	if state.input.Value() != "" {
		t.Fatal("draft was not restored after newest history entry")
	}
	m = typeText(m, "draft")
	m = update(m, tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = typeText(m, "echo")
	if !state.searching || !strings.Contains(m.View().Content, "echo session") {
		t.Fatal("reverse search did not find newest command")
	}
	m = update(m, tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if state.match != 0 {
		t.Fatalf("repeated reverse search did not find older saved command: %d", state.match)
	}
	m = press(m, tea.KeyEscape)
	if state.input.Value() != "draft" {
		t.Fatal("reverse search cancel did not restore draft")
	}
	state.input.SetValue("cd new")
	m, cmd = consoleKeyUpdate(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("cd did not validate remotely")
	}
	m = update(m, cmd())
	if state.identity.Cwd != "/srv/new" {
		t.Fatal("validated cwd not persisted in console")
	}
	m = press(m, tea.KeyEscape)
	if m.activeView != 0 || m.selected != "web-01" {
		t.Fatal("console return discarded Fleet selection")
	}
	backend.account = "deploy"
	m, cmd = consoleKeyUpdate(m, 's')
	if cmd == nil {
		t.Fatal("reopening console must verify execution account")
	}
	m = update(m, cmd())
	if state.identity.Account != "deploy" || len(state.history.Session) != 0 || len(state.entries) != 0 {
		t.Fatal("old account's transcript/history leaked to new execution account")
	}
	m = press(m, tea.KeyEscape)
	m = press(m, tea.KeyDown)
	m = press(m, 's')
	if m.consoleID != "web-02" || m.consoles[m.consoleKey("web-02")] == state {
		t.Fatal("console state leaked to another minion")
	}
}
