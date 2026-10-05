package app

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"saltrtui/internal/fleet"
	"saltrtui/internal/target"
	"saltrtui/internal/ui"
)

type targetState struct {
	input        textinput.Model
	mode         target.Mode
	revision     int
	request      int
	busy, stale  bool
	resultsFocus bool
	suggestion   int
	cursor       int
	offset       int
	preview      fleet.Observation[target.Preview]
	errorText    string
	notice       string
}

type targetLoaded struct {
	context       string
	request, edit int
	ids           []string
	at            time.Time
	err           error
}

type savedTarget struct {
	context string
	preview target.Preview
	at      time.Time
}

func (m *Model) openTarget() {
	if m.selected == "" {
		return
	}
	input := textinput.New()
	input.Prompt = "> "
	input.SetWidth(max(1, m.width-12))
	input.SetValue(m.selected)
	m.target = &targetState{input: input, mode: target.List}
	m.activeView = 3
	m.target.input.Focus()
}

func (m *Model) targetEdit() {
	s := m.target
	s.revision++
	s.stale = true
	s.errorText, s.notice = "", ""
	s.suggestion = 0
	s.resultsFocus = false
	s.offset = 0
}

func (m *Model) previewTarget() tea.Cmd {
	s := m.target
	if s == nil || s.busy {
		return nil
	}
	m.targetRequest++
	s.request = m.targetRequest
	s.busy, s.errorText, s.notice = true, "", ""
	ctxID, req, edit, backend := m.context, s.request, s.revision, m.backend
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
		defer cancel()
		ids, err := backend.ListAcceptedKeys(ctx)
		return targetLoaded{ctxID, req, edit, ids, time.Now(), err}
	}
}

func (m Model) targetResult(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	result, ok := msg.(targetLoaded)
	if !ok {
		return m, nil, false
	}
	s := m.target
	if s == nil || m.context != result.context || s.request != result.request {
		return m, nil, true
	}
	s.busy = false
	if result.edit != s.revision {
		return m, nil, true
	}
	s.preview.Err = result.err
	if result.err == nil {
		preview, err := target.Resolve(s.mode, s.input.Value(), fleet.SortUnique(result.ids))
		if err != nil {
			s.errorText = err.Error()
			s.stale = true
		} else {
			s.preview.Value, s.preview.At, s.errorText = preview, result.at, ""
			s.stale = false
			s.cursor, s.offset = 0, 0
		}
	}
	return m, nil, true
}

func (m Model) targetViewData() ui.TargetViewData {
	v := ui.TargetViewData{Width: m.width, Height: m.height, Context: m.context}
	s := m.target
	if s == nil {
		return v
	}
	v.Mode, v.Expression = string(s.mode), s.preview.Value.Expression
	v.Input, v.Offset, v.Cursor = s.input.View(), s.offset, s.cursor
	v.Suggestion, v.ResultsFocus, v.Busy, v.Stale = s.suggestion, s.resultsFocus, s.busy, s.stale
	v.Suggestions = target.Suggestions(s.mode, s.input.Value(), m.keys.Value)
	v.IDs, v.Preview = s.preview.Value.IDs, ui.Source{At: s.preview.At, Err: s.preview.Err, Busy: s.busy}
	v.Inventory = ui.Source{At: m.keys.At, Err: m.keys.Err, Busy: m.keys.Busy}
	v.Presence = ui.Source{At: m.presence.At, Err: m.presence.Err, Busy: m.presence.Busy}
	v.Connected = make(map[string]bool, len(m.presence.Value))
	for _, id := range m.presence.Value {
		v.Connected[id] = true
	}
	v.Error, v.Notice = s.errorText, s.notice
	v.Saved = m.savedTarget != nil && m.savedTarget.context == m.context && !s.stale && s.preview.At.Equal(m.savedTarget.at)
	return v
}

func (m *Model) insertTargetSuggestion() bool {
	s := m.target
	suggestions := target.Suggestions(s.mode, s.input.Value(), m.keys.Value)
	if len(suggestions) == 0 {
		return false
	}
	id := suggestions[min(s.suggestion, len(suggestions)-1)]
	value := s.input.Value()
	if s.mode == target.List {
		if i := strings.LastIndex(value, ","); i >= 0 {
			value = value[:i+1] + id
		} else {
			value = id
		}
	} else {
		value = id
	}
	if value == s.input.Value() {
		return false
	}
	s.input.SetValue(value)
	m.targetEdit()
	return true
}

func (m Model) updateTarget(msg tea.Msg) (tea.Model, tea.Cmd) {
	s := m.target
	if s == nil {
		m.activeView = 0
		return m, nil
	}
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		key := keyMsg.String()
		switch key {
		case "esc":
			m.activeView = 0
			s.input.Blur()
			return m, nil
		case "ctrl+t":
			if s.mode == target.List {
				s.mode = target.Glob
			} else {
				s.mode = target.List
			}
			m.targetEdit()
			return m, nil
		case "ctrl+p", "enter":
			if !s.resultsFocus || key == "ctrl+p" {
				return m, m.previewTarget()
			}
		case "ctrl+s":
			if s.busy || s.stale || s.preview.Err != nil || s.preview.At.IsZero() || len(s.preview.Value.IDs) == 0 {
				s.notice = "Preview a nonempty target before saving"
				return m, nil
			}
			m.savedTarget = &savedTarget{context: m.context, preview: s.preview.Value, at: s.preview.At}
			s.notice = "Target saved in memory; actions are not available yet"
			return m, nil
		case "tab":
			if s.resultsFocus {
				s.resultsFocus = false
				s.offset = 0
				return m, s.input.Focus()
			}
			if m.insertTargetSuggestion() {
				return m, nil
			}
			if len(s.preview.Value.IDs) > 0 {
				s.resultsFocus = true
				s.input.Blur()
				s.offset = ui.TargetCursorOffset(m.targetViewData())
			}
			return m, nil
		case "ctrl+n":
			if len(s.preview.Value.IDs) > 0 {
				s.resultsFocus = true
				s.input.Blur()
				s.offset = ui.TargetCursorOffset(m.targetViewData())
			}
			return m, nil
		case "x":
			if s.resultsFocus && !s.stale && s.cursor < len(s.preview.Value.IDs) {
				ids := append([]string(nil), s.preview.Value.IDs...)
				ids = append(ids[:s.cursor], ids[s.cursor+1:]...)
				s.mode = target.List
				s.input.SetValue(strings.Join(ids, ","))
				m.targetEdit()
				s.preview.Value = target.Preview{Mode: target.List, Expression: s.input.Value(), IDs: ids}
				s.stale = true // Re-preview the effective list against fresh keys.
				s.notice = "Removed ID; preview the effective specific-minion list again"
				return m, s.input.Focus()
			}
		}
		if s.resultsFocus {
			switch key {
			case "up", "k":
				s.cursor = max(0, s.cursor-1)
			case "down", "j":
				s.cursor = min(len(s.preview.Value.IDs)-1, s.cursor+1)
			case "pgup", "pgdown":
				s.cursor = max(0, min(len(s.preview.Value.IDs)-1, s.cursor+scrollDelta(key, max(1, m.height-7))))
			}
			s.offset = ui.TargetCursorOffset(m.targetViewData())
			return m, nil
		}
		if key == "up" || key == "down" {
			suggestions := target.Suggestions(s.mode, s.input.Value(), m.keys.Value)
			if len(suggestions) > 0 {
				if key == "up" {
					s.suggestion = max(0, s.suggestion-1)
				} else {
					s.suggestion = min(len(suggestions)-1, s.suggestion+1)
				}
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	old := s.input.Value()
	s.input, cmd = s.input.Update(msg)
	if s.input.Value() != old {
		m.targetEdit()
	}
	return m, cmd
}
