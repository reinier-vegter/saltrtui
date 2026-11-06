package app

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"saltrtui/internal/fleet"
	"saltrtui/internal/keys"
	"saltrtui/internal/ui"
)

type keyListLoaded struct {
	context string
	request int
	items   []keys.Key
	at      time.Time
	err     error
}
type keyDetailLoaded struct {
	context string
	request int
	key     keys.Key
	detail  keys.Detail
	at      time.Time
	err     error
}
type keyChanged struct {
	context string
	request int
	key     keys.Key
	action  keys.Action
	err     error
}
type keyDetailObservation struct {
	value   fleet.Observation[keys.Detail]
	request int
}
type keyState struct {
	search                                           textinput.Model
	list                                             fleet.Observation[[]keys.Key]
	details                                          map[keys.Key]keyDetailObservation
	selected                                         keys.Key
	confirm                                          keys.Action
	confirmedKey                                     keys.Key
	confirmedContext                                 string
	confirmFocus, confirmOffset, confirmDetailOffset int
	verifyKey                                        keys.Key
	verifyAction                                     keys.Action
	status                                           string
	busy                                             bool
	request, detailRequest, changeRequest            int
	focus, offset, detailOffset, helpOffset          int
}

func newKeyState() keyState {
	input := textinput.New()
	input.Prompt = "/ "
	input.Placeholder = "filter IDs or states"
	input.SetWidth(24)
	return keyState{search: input, details: make(map[keys.Key]keyDetailObservation)}
}

func (m *Model) openKeys() tea.Cmd {
	m.activeView = 6
	if m.keyBackend == nil {
		m.key.status = "key management unavailable"
		return nil
	}
	if m.key.list.At.IsZero() && !m.key.list.Busy {
		return m.loadKeys()
	}
	return nil
}

func (m *Model) loadKeys() tea.Cmd {
	if m.keyBackend == nil || m.key.list.Busy || m.key.busy {
		return nil
	}
	m.key.request++
	m.key.list.Busy = true
	m.cancelKeyConfirmation("Keys refreshed; confirmation canceled. Inspect the key again.")
	m.clampKeyViewport()
	backend, contextID, request := m.keyBackend, m.context, m.key.request
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
		defer cancel()
		items, err := backend.ListKeys(ctx)
		return keyListLoaded{contextID, request, items, time.Now(), err}
	}
}

func (m *Model) visibleKeys() []keys.Key { return keys.Filter(m.key.list.Value, m.key.search.Value()) }

func (m *Model) ambiguousKey() bool {
	count := 0
	for _, key := range m.key.list.Value {
		if key.ID == m.key.selected.ID {
			count++
		}
	}
	return count > 1
}

func (m *Model) alignKeySelection() {
	for _, key := range m.visibleKeys() {
		if key == m.key.selected {
			return
		}
	}
	items := m.visibleKeys()
	m.key.selected = keys.Key{}
	if len(items) > 0 {
		m.key.selected = items[0]
	}
	m.key.offset, m.key.detailOffset = 0, 0
	m.cancelKeyConfirmation("Key selection changed; confirmation canceled. Inspect the key again.")
}

func (m *Model) moveKey(delta int) {
	items := m.visibleKeys()
	if len(items) == 0 {
		return
	}
	idx := 0
	for i, key := range items {
		if key == m.key.selected {
			idx = i
			break
		}
	}
	idx = max(0, min(len(items)-1, idx+delta))
	if m.key.selected != items[idx] {
		m.key.detailOffset = 0
		m.cancelKeyConfirmation("Key selection changed; confirmation canceled. Inspect the key again.")
	}
	m.key.selected = items[idx]
	m.clampKeyViewport()
}

func (m *Model) inspectKey() tea.Cmd {
	key := m.key.selected
	if key.ID == "" || m.keyBackend == nil {
		return nil
	}
	m.key.focus = 1
	m.clampKeyViewport()
	entry := m.key.details[key]
	if entry.value.Busy {
		return nil
	}
	m.key.detailRequest++
	entry.request = m.key.detailRequest
	entry.value.Busy = true
	m.key.details[key] = entry
	backend, contextID, request := m.keyBackend, m.context, entry.request
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
		defer cancel()
		detail, err := backend.ReadKey(ctx, key)
		return keyDetailLoaded{contextID, request, key, detail, time.Now(), err}
	}
}

func (m *Model) cancelKeyConfirmation(status string) {
	if m.key.confirm == "" {
		return
	}
	m.key.confirm = ""
	m.key.status = status
	m.key.focus = m.key.confirmFocus
	m.key.offset = m.key.confirmOffset
	if m.key.selected == m.key.confirmedKey {
		m.key.detailOffset = m.key.confirmDetailOffset
	} else {
		m.key.detailOffset = 0
	}
	m.clampKeyViewport()
}

func (m *Model) clampKeyViewport() {
	rows := ui.KeysListRows(m.keysViewData())
	items := m.visibleKeys()
	m.key.offset = max(0, min(m.key.offset, max(0, len(items)-rows)))
	for i, key := range items {
		if key != m.key.selected {
			continue
		}
		if i < m.key.offset {
			m.key.offset = i
		} else if i >= m.key.offset+rows {
			m.key.offset = i - rows + 1
		}
		break
	}
	m.key.detailOffset = max(0, min(m.key.detailOffset, ui.KeysScrollLimit(m.keysViewData(), 1)))
	m.key.helpOffset = max(0, min(m.key.helpOffset, ui.KeysScrollLimit(m.keysViewData(), 2)))
}

func (m *Model) keyConfirmationIssue() string {
	key, action := m.key.confirmedKey, m.key.confirm
	switch {
	case m.keyBackend == nil:
		return "Key gateway unavailable"
	case m.key.confirmedContext != m.context:
		return "Master context changed"
	case m.key.list.Err != nil:
		return "Key inventory unavailable; refresh keys"
	case m.key.busy || m.key.list.Busy:
		return "Key inventory or action is busy; wait and inspect again"
	case key != m.key.selected:
		return "Key selection changed"
	case action == "" || key.State != action.Expected():
		return "Key state changed"
	}
	count := 0
	for _, item := range m.key.list.Value {
		if item.ID == key.ID {
			count++
			if item.State != key.State {
				return "Key state changed or ID is ambiguous; inspect again"
			}
		}
	}
	if count != 1 {
		return "Key missing or ID is ambiguous; refresh and inspect again"
	}
	return ""
}

func (m *Model) changeKey() tea.Cmd {
	key, action := m.key.confirmedKey, m.key.confirm
	if action == "" {
		return nil
	}
	if reason := m.keyConfirmationIssue(); reason != "" {
		m.cancelKeyConfirmation(reason + "; confirmation canceled. No action sent.")
		return nil
	}
	if !ui.KeysPromptLayout(m.keysViewData()).FitsConfirmation {
		m.key.status = "Resize to review the key and consequence; n/esc cancels"
		return nil
	}
	m.key.confirm = ""
	m.key.busy = true
	m.key.status = fmt.Sprintf("Checking %s before %s…", key.ID, action)
	m.clampKeyViewport()
	m.key.changeRequest++
	backend, contextID, request := m.keyBackend, m.context, m.key.changeRequest
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
		defer cancel()
		err := backend.ChangeKey(ctx, key, action)
		return keyChanged{contextID, request, key, action, err}
	}
}

func (m *Model) keysResult(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch value := msg.(type) {
	case keyListLoaded:
		if value.context != m.context || value.request != m.key.request {
			return *m, nil, true
		}
		m.key.list.Busy, m.key.list.Err = false, value.err
		if value.err == nil {
			m.key.list.Value, m.key.list.At = value.items, value.at
			if m.key.confirm != "" {
				if reason := m.keyConfirmationIssue(); reason != "" {
					m.cancelKeyConfirmation(reason + "; confirmation canceled. No action sent.")
				}
			}
			m.alignKeySelection()
		} else {
			m.cancelKeyConfirmation("Key inventory unavailable; confirmation canceled. Refresh keys.")
		}
		if m.key.verifyAction != "" {
			if value.err != nil {
				m.key.status = "Key action sent; verification unavailable. Refresh keys to check its state."
			} else {
				verified := m.key.verifyAction == keys.Revoke
				matches := 0
				for _, item := range value.items {
					if item.ID != m.key.verifyKey.ID {
						continue
					}
					matches++
					switch m.key.verifyAction {
					case keys.Accept:
						verified = item.State == keys.Accepted
					case keys.Block:
						verified = item.State == keys.Rejected
					case keys.Revoke:
						verified = verified && item.State != keys.Accepted
					}
				}
				if m.key.verifyAction != keys.Revoke {
					verified = verified && matches == 1
				}
				if verified {
					m.key.status = fmt.Sprintf("%s %s verified in refreshed keys", m.key.verifyAction, m.key.verifyKey.ID)
				} else {
					m.key.status = "Key action sent but expected state not observed; refresh and investigate."
				}
			}
			m.key.verifyAction = ""
		}
		m.clampKeyViewport()
		return *m, nil, true
	case keyDetailLoaded:
		entry, ok := m.key.details[value.key]
		if value.context != m.context || !ok || entry.request != value.request {
			return *m, nil, true
		}
		entry.value.Busy, entry.value.Err = false, value.err
		if value.err == nil {
			entry.value.Value = value.detail
			entry.value.At = value.at
		} else if entry.value.At.IsZero() {
			// A first failed fingerprint read may still provide a file-time hint.
			entry.value.Value = value.detail
		}
		m.key.details[value.key] = entry
		if m.key.selected == value.key {
			m.clampKeyViewport()
		}
		return *m, nil, true
	case keyChanged:
		if value.context != m.context || value.request != m.key.changeRequest {
			return *m, nil, true
		}
		m.key.busy = false
		if value.err != nil {
			m.key.status = value.err.Error()
		} else {
			m.key.status = fmt.Sprintf("%s %s sent; verifying refreshed keys…", value.action, value.key.ID)
			m.key.verifyKey, m.key.verifyAction = value.key, value.action
		}
		// Invalidate cached metadata, including the old key-file time.
		delete(m.key.details, value.key)
		return *m, tea.Batch(m.loadKeys(), m.refresh()), true
	}
	return *m, nil, false
}

func (m Model) keysViewData() ui.KeysViewData {
	contextID := m.context
	if m.key.confirm != "" {
		contextID = m.key.confirmedContext
	}
	entry := m.key.details[m.key.selected]
	rows := make([]ui.KeyRow, 0, len(m.visibleKeys()))
	for _, key := range m.visibleKeys() {
		rows = append(rows, ui.KeyRow{ID: key.ID, State: string(key.State), Selected: key == m.key.selected})
	}
	return ui.KeysViewData{Width: m.width, Height: m.height, Context: contextID, Focus: m.key.focus,
		Offset: m.key.offset, DetailOffset: m.key.detailOffset, HelpOffset: m.key.helpOffset,
		Query: m.key.search.Value(), Search: m.key.search.View(), Searching: m.key.search.Focused(), Help: m.help,
		Selected: ui.KeyRow{ID: m.key.selected.ID, State: string(m.key.selected.State)}, Rows: rows,
		ConfirmedKey: ui.KeyRow{ID: m.key.confirmedKey.ID, State: string(m.key.confirmedKey.State)},
		Ambiguous:    m.ambiguousKey(),
		List:         ui.Source{At: m.key.list.At, Err: m.key.list.Err, Busy: m.key.list.Busy},
		Detail:       ui.Source{At: entry.value.At, Err: entry.value.Err, Busy: entry.value.Busy},
		Fingerprint:  entry.value.Value.Fingerprint, FileTime: entry.value.Value.FileTime,
		Confirm: string(m.key.confirm), Busy: m.key.busy, BackendAvailable: m.keyBackend != nil, Status: m.key.status}
}

func (m Model) updateKeys(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		if m.key.confirm != "" {
			return m, nil
		}
		if m.key.search.Focused() {
			var cmd tea.Cmd
			m.key.search, cmd = m.key.search.Update(msg)
			m.alignKeySelection()
			m.clampKeyViewport()
			return m, cmd
		}
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.help {
			if key == "esc" || key == "?" || key == "q" {
				m.help = false
				m.clampKeyViewport()
			} else {
				m.key.helpOffset = max(0, min(m.key.helpOffset+scrollDelta(key, ui.KeysPageRows(m.keysViewData())), ui.KeysScrollLimit(m.keysViewData(), 2)))
			}
			return m, nil
		}
		if m.key.confirm != "" {
			if key == "y" {
				return m, m.changeKey()
			}
			if key == "esc" || key == "n" {
				m.cancelKeyConfirmation("Key action canceled; no action sent")
			}
			return m, nil
		}
		if m.key.search.Focused() {
			if key == "esc" || key == "enter" {
				m.key.search.Blur()
				m.clampKeyViewport()
				return m, nil
			}
			var cmd tea.Cmd
			m.key.search, cmd = m.key.search.Update(msg)
			m.alignKeySelection()
			m.clampKeyViewport()
			return m, cmd
		}
		if cmd, handled := m.switchWorkspace(key); handled {
			return m, cmd
		}
		switch key {
		case "q":
			m.stopEvents()
			return m, tea.Quit
		case "esc":
			if m.key.focus == 1 {
				m.key.focus = 0
				m.clampKeyViewport()
				return m, nil
			}
			m.activeView = 0
		case "?":
			m.help = true
			m.key.helpOffset = 0
			m.clampKeyViewport()
		case "/":
			if m.key.focus == 0 {
				cmd := m.key.search.Focus()
				m.clampKeyViewport()
				return m, cmd
			}
		case "tab", "shift+tab":
			m.key.focus = 1 - m.key.focus
			m.clampKeyViewport()
		case "r":
			if !m.key.busy && !m.key.list.Busy {
				m.key.status = ""
			}
			return m, m.loadKeys()
		case "enter":
			cmd := m.inspectKey()
			m.clampKeyViewport()
			return m, cmd
		case "A", "b", "d":
			if m.key.busy || m.key.list.Busy || m.key.list.Err != nil || m.key.selected.ID == "" || m.ambiguousKey() {
				break
			}
			action := map[string]keys.Action{"A": keys.Accept, "b": keys.Block, "d": keys.Revoke}[key]
			if m.key.selected.State == action.Expected() {
				m.key.confirmFocus, m.key.confirmOffset, m.key.confirmDetailOffset = m.key.focus, m.key.offset, m.key.detailOffset
				m.key.confirmedKey, m.key.confirm = m.key.selected, action
				m.key.confirmedContext, m.key.status = m.context, ""
			}
		case "up", "down", "pgup", "pgdown":
			delta := scrollDelta(key, ui.KeysPageRows(m.keysViewData()))
			if m.key.focus == 0 {
				m.moveKey(delta)
			} else {
				m.key.detailOffset = max(0, min(m.key.detailOffset+delta, ui.KeysScrollLimit(m.keysViewData(), 1)))
			}
		}
	}
	return m, nil
}
