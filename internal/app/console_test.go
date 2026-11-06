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

func consoleDraftModel(t *testing.T) (Model, *consoleState, *fakeConsole) {
	t.Helper()
	backend := &fakeConsole{}
	m := NewWithConsole(fakeGateway{}, fakeJobs{}, backend, "master-a", true)
	m.width, m.height, m.selected = 120, 25, "web-01"
	cmd := m.openConsole()
	m = update(m, cmd())
	state := m.consoles[m.consoleKey(m.consoleID)]
	m = update(m, consoleHistoryLoaded{context: m.context, id: m.consoleID, request: state.setupRequest, items: []string{"echo saved", "pwd"}})
	return m, state, backend
}

func clearConsoleDraft(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("console Ctrl-C quit the application")
		}
	}
	return next.(Model)
}

func TestConsoleCtrlCClearsEditingOnly(t *testing.T) {
	for _, mode := range []string{"draft", "recalled", "search match", "search no match", "empty", "loading"} {
		t.Run(mode, func(t *testing.T) {
			m, state, backend := consoleDraftModel(t)
			defer m.Close()
			state.history.Add("echo session")
			state.entries = []consoleEntry{{command: "pwd", result: console.Result{Stdout: "/srv"}}}
			state.offset = 3
			other := &consoleState{searchDraft: "untouched"}
			m.consoles[m.consoleKey("web-02")] = other
			m = typeText(m, "discard me")
			switch mode {
			case "recalled":
				m = press(m, tea.KeyUp)
			case "search match", "search no match":
				m = update(m, tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
				query := "echo"
				if mode == "search no match" {
					query = "no-such-command"
				}
				m = typeText(m, query)
			case "empty":
				state.input.SetValue("")
			case "loading":
				state.identityBusy, state.histBusy = true, true
			}
			identity := state.identity
			request, setupRequest := state.request, state.setupRequest
			m = clearConsoleDraft(t, m)
			m = clearConsoleDraft(t, m)
			if m.activeView != 2 || m.consoleID != "web-01" || m.selected != "web-01" || state.input.Value() != "" || !state.input.Focused() {
				t.Fatal("Ctrl-C did not leave a focused empty console")
			}
			if state.searching || state.reverse.Value() != "" || state.reverse.Focused() || state.searchDraft != "" || state.match != -1 {
				t.Fatal("reverse-search state survived cancellation")
			}
			m = press(m, tea.KeyDown)
			if state.input.Value() != "" || len(state.history.Saved) != 2 || len(state.history.Session) != 1 {
				t.Fatal("canceled draft returned or history changed")
			}
			if state.identity != identity || state.request != request || state.setupRequest != setupRequest || state.offset != 3 || len(state.entries) != 1 || state.entries[0].command != "pwd" || other.searchDraft != "untouched" || len(backend.calls) != 0 {
				t.Fatal("Ctrl-C changed non-editing state or dispatched a command")
			}
			if mode == "loading" {
				if !state.identityBusy || !state.histBusy {
					t.Fatal("Ctrl-C canceled setup")
				}
				state.identityBusy, state.histBusy = false, false
			}
			m = typeText(m, "echo replacement")
			m, cmd := consoleKeyUpdate(m, tea.KeyEnter)
			if cmd == nil || state.input.Value() != "" || !state.busy {
				t.Fatal("replacement command could not be submitted")
			}
			m = update(m, cmd())
			if state.busy || len(backend.calls) != 1 || backend.calls[0] != "web-01:/srv:echo replacement" {
				t.Fatal("replacement dispatch/result failed")
			}
			m = press(m, tea.KeyEscape)
			if m.activeView != 0 || m.selected != "web-01" {
				t.Fatal("Escape no longer returns to Fleet")
			}
		})
	}
}

func TestConsoleCtrlCPreservesPendingCommand(t *testing.T) {
	m, state, backend := consoleDraftModel(t)
	defer m.Close()
	m = typeText(m, "echo pending")
	m, pending := consoleKeyUpdate(m, tea.KeyEnter)
	request := state.request
	m = typeText(m, "discard")
	m = clearConsoleDraft(t, m)
	if !state.busy || state.request != request || len(state.history.Session) != 1 {
		t.Fatal("Ctrl-C canceled or changed pending request")
	}
	m = typeText(m, "echo next")
	m, cmd := consoleKeyUpdate(m, tea.KeyEnter)
	if cmd != nil || state.input.Value() != "echo next" || len(backend.calls) != 0 {
		t.Fatal("replacement dispatched while a command was pending")
	}
	m = update(m, pending())
	if state.busy || len(state.entries) != 1 || state.entries[0].command != "echo pending" || state.input.Value() != "echo next" {
		t.Fatal("pending result or replacement draft lost")
	}
	m, cmd = consoleKeyUpdate(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatal("replacement blocked after pending result")
	}
	m = update(m, cmd())
	if len(backend.calls) != 2 || state.busy {
		t.Fatal("replacement did not complete")
	}
}

func TestConsoleCtrlCRoutingBoundaries(t *testing.T) {
	for _, route := range []string{"missing", "Fleet", "graph", "assignments", "update", "installing"} {
		t.Run(route, func(t *testing.T) {
			m, state, _ := consoleDraftModel(t)
			defer m.Close()
			state.input.SetValue("keep")
			stoppedEvents, stoppedGraph := false, false
			m.event.cancel = func() { stoppedEvents = true }
			m.graph.cancel = func() { stoppedGraph = true }
			switch route {
			case "missing":
				delete(m.consoles, m.consoleKey(m.consoleID))
			case "Fleet":
				m.activeView = 0
			case "graph":
				m.graph.open = true
			case "assignments":
				m.activeView = 9
				m.assignments = newAssignmentsState()
			case "update", "installing":
				m.activeView = 8
				m.updatePhase = route
			}
			next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
			m = next.(Model)
			if route == "missing" || route == "installing" || route == "assignments" {
				if cmd != nil || stoppedEvents || stoppedGraph {
					t.Fatal("safe fallback quit or canceled background work")
				}
				if route == "missing" && m.activeView != 0 || route == "installing" && m.activeView != 8 {
					t.Fatal("wrong fallback route")
				}
				if route == "assignments" && m.activeView != 9 {
					t.Fatal("assignments Ctrl-C behavior changed")
				}
			} else {
				if cmd == nil {
					t.Fatal("existing quit behavior lost")
				}
				if _, quit := cmd().(tea.QuitMsg); !quit || !stoppedEvents {
					t.Fatal("existing quit/cleanup behavior lost")
				}
				if (route == "Fleet" || route == "graph") && !stoppedGraph {
					t.Fatal("global quit did not stop graph")
				}
			}
			if state.input.Value() != "keep" {
				t.Fatal("non-console route cleared console draft")
			}
		})
	}
}
