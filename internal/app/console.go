package app

import (
	"context"
	"errors"
	"path"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"saltrtui/internal/console"
	"saltrtui/internal/ui"
)

type consoleEntry struct {
	command string
	result  console.Result
	err     error
	at      time.Time
}

const storedConsoleOutput = 12 << 10

func boundedConsoleOutput(text string) string {
	if len(text) <= storedConsoleOutput {
		return text
	}
	return strings.ToValidUTF8(text[:storedConsoleOutput], "") + "\n… (output truncated in console)"
}

type consoleState struct {
	input, reverse               textinput.Model
	identity                     console.Identity
	history                      console.HistoryState
	entries                      []consoleEntry
	request                      int
	setupRequest                 int
	busy, identityBusy, histBusy bool
	identityErr, historyErr      error
	offset, match                int
	searchDraft                  string
	searching                    bool
}

type consoleIdentityLoaded struct {
	context, id string
	request     int
	identity    console.Identity
	err         error
}
type consoleHistoryLoaded struct {
	context, id string
	request     int
	items       []string
	err         error
}
type consoleCommandLoaded struct {
	context, id, line string
	request           int
	result            console.Result
	cwd               string
	change            bool
	err               error
}

func (m Model) consoleKey(id string) string { return m.context + "\x00" + id }

func (m *Model) openConsole() tea.Cmd {
	if m.selected == "" || m.consoleBackend == nil {
		return nil
	}
	id := m.selected
	m.consoleID, m.activeView = id, 2
	key := m.consoleKey(id)
	if state := m.consoles[key]; state != nil {
		if state.identityBusy || state.busy {
			return nil
		}
		state.setupRequest++
		state.identityBusy = true
		ctxID, backend, req := m.context, m.consoleBackend, state.setupRequest
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 24*time.Second)
			defer cancel()
			identity, err := backend.Identity(ctx, id)
			return consoleIdentityLoaded{ctxID, id, req, identity, err}
		}
	}
	input := textinput.New()
	input.Prompt = "$ "
	input.Placeholder = "one finite command"
	input.SetWidth(max(1, m.width-8))
	input.Focus()
	reverse := textinput.New()
	reverse.Prompt = "reverse search: "
	reverse.SetWidth(max(1, m.width-20))
	state := &consoleState{input: input, reverse: reverse, identityBusy: true, match: -1, setupRequest: 1}
	m.consoles[key] = state
	ctxID, backend, req := m.context, m.consoleBackend, state.setupRequest
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 24*time.Second)
		defer cancel()
		identity, err := backend.Identity(ctx, id)
		return consoleIdentityLoaded{ctxID, id, req, identity, err}
	}
}

func (m Model) consoleResult(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch result := msg.(type) {
	case consoleIdentityLoaded:
		state := m.consoles[m.consoleKey(result.id)]
		if m.context != result.context || state == nil || state.setupRequest != result.request {
			return m, nil, true
		}
		if state.identity.Account != "" && state.identity.Account != result.identity.Account {
			state.history = console.HistoryState{}
			state.entries = nil
			state.input.SetValue("")
			state.offset = 0
		}
		if state.identity.Cwd != "" && state.identity.Account == result.identity.Account {
			result.identity.Cwd = state.identity.Cwd
		}
		state.identityBusy, state.identityErr, state.identity = false, result.err, result.identity
		if result.identity.Home == "" {
			return m, nil, true
		}
		state.histBusy = true
		ctxID, id, req, backend, home := m.context, result.id, state.setupRequest, m.consoleBackend, result.identity.Home
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
			defer cancel()
			items, err := backend.History(ctx, id, home)
			return consoleHistoryLoaded{ctxID, id, req, items, err}
		}, true
	case consoleHistoryLoaded:
		state := m.consoles[m.consoleKey(result.id)]
		if m.context == result.context && state != nil && state.setupRequest == result.request {
			state.histBusy, state.historyErr = false, result.err
			if result.err == nil {
				state.history.Saved = result.items
				state.history.ResetNavigation()
			}
		}
		return m, nil, true
	case consoleCommandLoaded:
		state := m.consoles[m.consoleKey(result.id)]
		if m.context == result.context && state != nil && state.request == result.request && state.busy {
			state.busy = false
			result.result.Stdout = boundedConsoleOutput(result.result.Stdout)
			result.result.Stderr = boundedConsoleOutput(result.result.Stderr)
			if result.change && result.err == nil {
				state.identity.Cwd = result.cwd
			}
			state.entries = append(state.entries, consoleEntry{command: result.line, result: result.result, err: result.err, at: time.Now()})
			if len(state.entries) > 100 {
				state.entries = state.entries[len(state.entries)-100:]
			}
			state.offset = ui.ConsoleScrollLimit(m.consoleViewData(result.id))
		}
		return m, nil, true
	}
	return m, nil, false
}

func cdArgument(line, home, cwd string) (string, error) {
	arg := strings.TrimSpace(strings.TrimPrefix(line, "cd"))
	if arg == "" {
		arg = home
	}
	if len(arg) >= 2 && (arg[0] == '\'' && arg[len(arg)-1] == '\'' || arg[0] == '"' && arg[len(arg)-1] == '"') {
		arg = arg[1 : len(arg)-1]
	}
	if (arg == "~" || strings.HasPrefix(arg, "~/")) && home == "" {
		return "", errors.New("account home unknown; use an absolute directory")
	}
	if arg == "~" || strings.HasPrefix(arg, "~/") {
		arg = path.Join(home, strings.TrimPrefix(arg, "~"))
	}
	if arg == "" || strings.ContainsAny(arg, "\n\r\x00") || strings.ContainsAny(arg, "'\"`$;|&<>*") {
		return "", errors.New("cd accepts a literal directory path (optionally quoted)")
	}
	if !path.IsAbs(arg) {
		if cwd == "" {
			return "", errors.New("current directory unknown; use an absolute path")
		}
		arg = path.Join(cwd, arg)
	}
	return path.Clean(arg), nil
}

func (m *Model) runConsole(line string) tea.Cmd {
	state := m.consoles[m.consoleKey(m.consoleID)]
	line = strings.TrimSpace(line)
	if state == nil || state.busy || state.identityBusy || line == "" {
		return nil
	}
	if len(line) > 8192 || strings.ContainsAny(line, "\r\x00") {
		state.entries = append(state.entries, consoleEntry{command: "(invalid input)", err: errors.New("command exceeds 8192 bytes or contains invalid control characters")})
		return nil
	}
	state.history.Add(line)
	state.input.SetValue("")
	state.busy = true
	state.request++
	ctxID, id, req, backend, cwd := m.context, m.consoleID, state.request, m.consoleBackend, state.identity.Cwd
	change := line == "cd" || strings.HasPrefix(line, "cd ")
	if change {
		dir, err := cdArgument(line, state.identity.Home, cwd)
		if err != nil {
			state.busy = false
			state.entries = append(state.entries, consoleEntry{command: line, err: err})
			return nil
		}
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
			defer cancel()
			resolved, err := backend.ChangeDirectory(ctx, id, dir)
			return consoleCommandLoaded{context: ctxID, id: id, request: req, line: line, cwd: resolved, change: true, err: err}
		}
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
		defer cancel()
		result, err := backend.Run(ctx, id, line, cwd)
		return consoleCommandLoaded{context: ctxID, id: id, request: req, line: line, result: result, err: err}
	}
}

func (m Model) updateConsole(msg tea.Msg) (tea.Model, tea.Cmd) {
	state := m.consoles[m.consoleKey(m.consoleID)]
	if state == nil {
		m.activeView = 0
		return m, nil
	}
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		key := keyMsg.String()
		if state.searching {
			switch key {
			case "esc":
				state.searching = false
				state.reverse.Blur()
				state.input.SetValue(state.searchDraft)
				return m, nil
			case "enter":
				state.searching = false
				state.reverse.Blur()
				if state.match < 0 {
					state.input.SetValue(state.searchDraft)
					return m, nil
				}
				_, match := state.history.Reverse(state.reverse.Value(), state.match+1)
				if state.busy {
					state.input.SetValue(match)
					return m, nil
				}
				return m, m.runConsole(match)
			case "ctrl+r":
				if older, _ := state.history.Reverse(state.reverse.Value(), state.match); older >= 0 {
					state.match = older
				}
				return m, nil
			}
			var cmd tea.Cmd
			state.reverse, cmd = state.reverse.Update(msg)
			state.match, _ = state.history.Reverse(state.reverse.Value(), state.history.Len())
			return m, cmd
		}
		switch key {
		case "esc":
			m.activeView = 0
		case "ctrl+r":
			state.searchDraft, state.searching = state.input.Value(), true
			state.reverse.SetValue("")
			state.match, _ = state.history.Reverse("", state.history.Len())
			return m, state.reverse.Focus()
		case "up":
			state.input.SetValue(state.history.Up(state.input.Value()))
		case "down":
			state.input.SetValue(state.history.Down(state.input.Value()))
		case "pgup", "pgdown":
			state.offset = max(0, min(state.offset+scrollDelta(key, max(1, m.height-7)), ui.ConsoleScrollLimit(m.consoleViewData(m.consoleID))))
		case "enter":
			return m, m.runConsole(state.input.Value())
		default:
			state.history.ResetNavigation()
			var cmd tea.Cmd
			state.input, cmd = state.input.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	if state.searching {
		var cmd tea.Cmd
		state.reverse, cmd = state.reverse.Update(msg)
		state.match, _ = state.history.Reverse(state.reverse.Value(), state.history.Len())
		return m, cmd
	}
	state.history.ResetNavigation()
	var cmd tea.Cmd
	state.input, cmd = state.input.Update(msg)
	return m, cmd
}

func (m Model) consoleViewData(id string) ui.ConsoleViewData {
	data := ui.ConsoleViewData{Width: m.width, Height: m.height, Context: m.context, ID: id}
	state := m.consoles[m.consoleKey(id)]
	if state == nil {
		return data
	}
	data.Account, data.Cwd, data.Input = state.identity.Account, state.identity.Cwd, state.input.View()
	data.Busy, data.IdentityBusy, data.HistoryBusy = state.busy, state.identityBusy, state.histBusy
	data.IdentityErr, data.HistoryErr, data.Offset = state.identityErr, state.historyErr, state.offset
	data.Saved, data.Session = len(state.history.Saved), len(state.history.Session)
	if state.searching {
		data.Search, data.Searching = state.reverse.View(), true
		if state.match >= 0 {
			_, data.Match = state.history.Reverse(state.reverse.Value(), state.match+1)
		}
	}
	for _, entry := range state.entries {
		data.Entries = append(data.Entries, ui.ConsoleEntry{Command: entry.command, Stdout: entry.result.Stdout, Stderr: entry.result.Stderr, Retcode: entry.result.Retcode, Err: entry.err, At: entry.at})
	}
	return data
}
