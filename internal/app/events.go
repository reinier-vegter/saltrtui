package app

import (
	"context"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"saltrtui/internal/events"
	"saltrtui/internal/ui"
)

type eventsStarted struct {
	context string
	request int
	stream  <-chan events.Update
	err     error
}

type eventNext struct {
	context string
	request int
	update  events.Update
	ok      bool
}

type eventState struct {
	search                          textinput.Model
	buffer                          events.Buffer
	stream                          <-chan events.Update
	lifetime                        context.Context
	lifetimeCancel                  context.CancelFunc
	ctx                             context.Context
	cancel                          context.CancelFunc
	family, status                  string
	focus, offset, detailOffset     int
	helpOffset, selected, inspect   int
	paused                          bool
	unseen, streamDropped, lossBase uint64
}

func newEventState() eventState {
	input := textinput.New()
	input.Prompt = "/ "
	input.Placeholder = "filter events"
	input.SetWidth(24)
	return eventState{search: input, family: "all", selected: -1, inspect: -1}
}

func (m *Model) stopEvents() {
	if m.event.cancel != nil {
		m.event.cancel()
		m.event.cancel = nil
	}
	m.event.stream = nil
	m.event.ctx = nil
}

func (m *Model) openEvents() tea.Cmd {
	m.activeView = 4
	if m.eventBackend == nil {
		m.event.status = "event listener unavailable"
	}
	return nil
}

func (m Model) eventStartCmd() tea.Cmd {
	if m.eventBackend == nil || m.event.ctx == nil {
		return nil
	}
	ctx := m.event.ctx
	backend, contextID, req := m.eventBackend, m.context, m.eventGeneration
	return func() tea.Msg {
		stream, err := backend.Subscribe(ctx)
		return eventsStarted{contextID, req, stream, err}
	}
}

func (m *Model) restartEvents() tea.Cmd {
	m.stopEvents()
	m.eventGeneration++
	m.event.lossBase = m.event.streamDropped
	m.event.status = "Listener restarted; events during this gap cannot be replayed"
	if m.eventBackend == nil {
		m.event.status = "event listener unavailable"
		return nil
	}
	m.event.ctx, m.event.cancel = context.WithCancel(m.event.lifetime)
	return m.eventStartCmd()
}

func waitEvent(stream <-chan events.Update, contextID string, request int) tea.Cmd {
	return func() tea.Msg {
		value, ok := <-stream
		return eventNext{contextID, request, value, ok}
	}
}

func (m *Model) eventIndices() []int {
	indices := make([]int, 0, len(m.event.buffer.Records))
	for i := len(m.event.buffer.Records) - 1; i >= 0; i-- {
		if events.Match(m.event.buffer.Records[i], m.event.family, m.event.search.Value()) {
			indices = append(indices, i)
		}
	}
	return indices
}

func (m *Model) alignEvent() {
	indices := m.eventIndices()
	for _, i := range indices {
		if i == m.event.selected {
			return
		}
	}
	m.event.selected = -1
	if len(indices) > 0 {
		m.event.selected = indices[0]
	}
	m.event.offset = 0
	m.event.inspect = -1
	m.event.detailOffset = 0
}

func (m *Model) eventMove(delta int) {
	indices := m.eventIndices()
	if len(indices) == 0 {
		return
	}
	idx := 0
	for i, value := range indices {
		if value == m.event.selected {
			idx = i
			break
		}
	}
	idx = max(0, min(len(indices)-1, idx+delta))
	m.event.selected = indices[idx]
	m.event.paused = true
	m.event.inspect = -1
	m.event.detailOffset = 0
	rows := ui.ListRows(m.height)
	if idx < m.event.offset {
		m.event.offset = idx
	} else if idx >= m.event.offset+rows {
		m.event.offset = idx - rows + 1
	}
}

func (m *Model) eventsResult(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch value := msg.(type) {
	case eventsStarted:
		if value.context != m.context || value.request != m.eventGeneration || m.event.cancel == nil {
			return *m, nil, true
		}
		if value.err != nil {
			m.event.status = value.err.Error()
			m.stopEvents()
			return *m, nil, true
		}
		if value.stream == nil {
			m.event.status = "event listener returned no stream; press r to retry"
			m.stopEvents()
			return *m, nil, true
		}
		m.event.stream = value.stream
		return *m, waitEvent(value.stream, m.context, m.eventGeneration), true
	case eventNext:
		if value.context != m.context || value.request != m.eventGeneration || m.event.cancel == nil {
			return *m, nil, true
		}
		if total := m.event.lossBase + value.update.Dropped; total > m.event.streamDropped {
			m.event.streamDropped = total
		}
		if !value.ok || value.update.Err != nil {
			m.event.status = "listener disconnected; events during this gap cannot be replayed"
			if value.update.Err != nil {
				m.event.status = value.update.Err.Error() + "; events during this gap cannot be replayed"
			}
			m.stopEvents()
			return *m, nil, true
		}
		oldSelected, oldPosition := m.event.selected, -1
		if m.event.paused {
			for i, idx := range m.eventIndices() {
				if idx == oldSelected {
					oldPosition = i
					break
				}
			}
		}
		before := m.event.buffer.Dropped
		m.event.buffer.Add(value.update.Record)
		removed := int(m.event.buffer.Dropped - before)
		m.event.selected -= removed
		m.event.inspect -= removed
		if m.event.selected < 0 || m.event.inspect < 0 {
			m.event.inspect = -1
		}
		if m.event.paused {
			m.event.unseen++
			m.alignEventIfMissing()
			if oldPosition >= 0 && m.event.selected == oldSelected-removed {
				for i, idx := range m.eventIndices() {
					if idx == m.event.selected {
						m.event.offset = max(0, m.event.offset+i-oldPosition)
						break
					}
				}
			}
			m.event.offset = min(m.event.offset, max(0, len(m.eventIndices())-ui.ListRows(m.height)))
		} else {
			m.event.selected = -1
			m.alignEvent()
		}
		return *m, waitEvent(m.event.stream, m.context, m.eventGeneration), true
	}
	return *m, nil, false
}

func (m *Model) alignEventIfMissing() {
	for _, i := range m.eventIndices() {
		if i == m.event.selected {
			return
		}
	}
	m.alignEvent()
}

func (m Model) eventsViewData() ui.EventsViewData {
	indices := m.eventIndices()
	rows := make([]ui.EventRow, 0, len(indices))
	selected, inspected := -1, -1
	for i, idx := range indices {
		record := m.event.buffer.Records[idx]
		rows = append(rows, ui.EventRow{Tag: record.Tag, Minion: record.Minion, JID: record.JID,
			Stamp: record.Stamp, Data: record.Data, Observed: record.Observed})
		if idx == m.event.selected {
			selected = i
		}
		if idx == m.event.inspect {
			inspected = i
		}
	}
	return ui.EventsViewData{Width: m.width, Height: m.height, Context: m.context, Family: m.event.family,
		Query: m.event.search.Value(), Search: m.event.search.View(), Searching: m.event.search.Focused(),
		Focus: m.event.focus, Offset: m.event.offset, DetailOffset: m.event.detailOffset,
		HelpOffset: m.event.helpOffset, Help: m.help, Status: m.event.status, Paused: m.event.paused,
		Unseen: m.event.unseen, Dropped: m.event.buffer.Dropped + m.event.streamDropped,
		Selected: selected, Inspected: inspected, Rows: rows}
}

func (m Model) updateEvents(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		if m.event.search.Focused() {
			var cmd tea.Cmd
			m.event.search, cmd = m.event.search.Update(msg)
			m.alignEvent()
			return m, cmd
		}
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "ctrl+c" {
			m.stopEvents()
			return m, tea.Quit
		}
		if m.help {
			if key == "esc" || key == "?" || key == "q" {
				m.help = false
			} else {
				delta := scrollDelta(key, ui.PageRows(m.height, 1, false))
				m.event.helpOffset = max(0, min(m.event.helpOffset+delta, ui.EventsScrollLimit(m.eventsViewData(), 2)))
			}
			return m, nil
		}
		if m.event.search.Focused() {
			if key == "enter" || key == "esc" {
				m.event.search.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.event.search, cmd = m.event.search.Update(msg)
			m.alignEvent()
			return m, cmd
		}
		switch key {
		case "q":
			m.stopEvents()
			return m, tea.Quit
		case "1", "esc":
			if key == "esc" && m.event.focus == 1 {
				m.event.focus = 0
				return m, nil
			}
			m.activeView = 0
		case "2":
			m.activeView = 1
			if m.jobList.At.IsZero() && m.jobList.Err == nil {
				return m, m.loadJobs()
			}
		case "3":
			return m, m.openAssignments()
		case "6":
			return m, m.openKeys()
		case "?":
			m.help = true
			m.event.helpOffset = 0
		case "/":
			if m.event.focus == 0 {
				return m, m.event.search.Focus()
			}
		case "tab", "shift+tab":
			m.event.focus = 1 - m.event.focus
		case "a", "g", "m", "K", "p":
			m.event.family = map[string]string{"a": "all", "g": "jobs", "m": "minions", "K": "keys", "p": "presence"}[key]
			m.alignEvent()
		case "space", " ":
			m.event.paused = !m.event.paused
			if !m.event.paused {
				m.event.unseen = 0
				m.event.selected = -1
				m.alignEvent()
			}
		case "c":
			m.event.buffer.Clear()
			m.event.selected, m.event.inspect = -1, -1
			m.event.offset, m.event.detailOffset = 0, 0
			m.event.unseen = 0
		case "r":
			return m, m.restartEvents()
		case "enter":
			if m.event.selected >= 0 {
				m.event.paused = true
				m.event.inspect = m.event.selected
				m.event.focus = 1
				m.event.detailOffset = 0
			}
		case "up", "down", "k", "j", "pgup", "pgdown":
			delta := scrollDelta(key, ui.PageRows(m.height, 1, false))
			if m.event.focus == 0 {
				m.eventMove(delta)
			} else {
				m.event.detailOffset = max(0, min(m.event.detailOffset+delta, ui.EventsScrollLimit(m.eventsViewData(), 1)))
			}
		}
	}
	return m, nil
}
