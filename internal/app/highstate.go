package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"saltrtui/internal/fleet"
	"saltrtui/internal/highstate"
	"saltrtui/internal/ui"
)

type highstateLoaded struct {
	context, id string
	request     int
	preview     bool
	report      highstate.Report
	at          time.Time
	err         error
}

type highstateState struct {
	context, id     string
	request         int
	preview         fleet.Observation[highstate.Report]
	apply           fleet.Observation[highstate.Report]
	busy, confirm   bool
	attempted, help bool
	status          string
	offset          int
}

const highstatePreviewLifetime = 15 * time.Minute

func (m *Model) openHighstate() tea.Cmd {
	if m.selected == "" || m.keys.At.IsZero() || m.keys.Busy || m.keys.Err != nil {
		return nil
	}
	found := false
	for _, id := range m.keys.Value {
		found = found || id == m.selected
	}
	if !found {
		return nil
	}
	m.highstateRequest++
	m.highstate = &highstateState{context: m.context, id: m.selected, request: m.highstateRequest}
	m.activeView = 7
	if m.highstateBackend == nil {
		m.highstate.status = "Highstate unavailable; no gateway configured"
	}
	return nil
}

func (m *Model) canApplyHighstate() bool {
	s := m.highstate
	if s == nil || m.highstateBackend == nil || s.context != m.context || s.id != m.selected || s.busy || s.attempted || s.preview.At.IsZero() || s.preview.Err != nil || s.preview.Value.Failed > 0 || len(s.preview.Value.Steps) == 0 || time.Since(s.preview.At) > highstatePreviewLifetime {
		return false
	}
	for _, id := range m.keys.Value {
		if id == s.id && m.keys.Err == nil {
			return true
		}
	}
	return false
}

func (m *Model) previewHighstate() tea.Cmd {
	s := m.highstate
	if s == nil || s.busy || m.highstateBackend == nil || s.context != m.context || s.id != m.selected {
		return nil
	}
	m.highstateRequest++
	s.request = m.highstateRequest
	s.preview, s.apply = fleet.Observation[highstate.Report]{Busy: true}, fleet.Observation[highstate.Report]{}
	s.busy, s.attempted, s.confirm, s.status, s.offset = true, false, false, "", 0
	backend, ctxID, id, req, parent := m.highstateBackend, m.context, s.id, s.request, m.workCtx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 4*time.Minute)
		defer cancel()
		report, err := backend.Preview(ctx, id)
		return highstateLoaded{ctxID, id, req, true, report, time.Now(), err}
	}
}

func (m *Model) applyHighstate() tea.Cmd {
	s := m.highstate
	if !m.canApplyHighstate() || !s.confirm || m.highstateBackend == nil {
		return nil
	}
	m.highstateRequest++
	s.request = m.highstateRequest
	s.busy, s.confirm, s.attempted = true, false, true
	s.apply = fleet.Observation[highstate.Report]{Busy: true}
	s.status = "Rechecking accepted key before dispatch…"
	backend, ctxID, id, req, parent := m.highstateBackend, m.context, s.id, s.request, m.workCtx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 20*time.Minute)
		defer cancel()
		report, err := backend.Apply(ctx, id)
		return highstateLoaded{ctxID, id, req, false, report, time.Now(), err}
	}
}

func (m Model) highstateResult(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	result, ok := msg.(highstateLoaded)
	if !ok {
		return m, nil, false
	}
	s := m.highstate
	if s == nil || !s.busy || result.context != m.context || s.context != result.context || result.id != s.id || result.request != s.request || result.preview != s.preview.Busy {
		return m, nil, true
	}
	s.busy = false
	if result.preview {
		s.preview.Busy, s.preview.Err = false, result.err
		if result.err != nil {
			s.status = "Preview unavailable; no Apply was sent: " + result.err.Error()
		} else {
			s.preview.Value, s.preview.At = result.report, result.at
			if len(result.report.Steps) == 0 {
				s.status = "Preview returned no state results; Apply disabled"
			} else if result.report.Failed > 0 {
				s.status = "Preview reported failed states; Apply disabled"
			} else {
				s.status = "Preview returned; inspect proposed changes before Apply"
			}
		}
	} else {
		s.apply.Busy, s.apply.Err = false, result.err
		if result.err != nil {
			if errors.Is(result.err, highstate.ErrNotSent) {
				s.status = "Apply not sent: " + result.err.Error()
			} else {
				s.status = "Apply outcome unknown; do not retry automatically: " + result.err.Error()
			}
		} else {
			s.apply.Value, s.apply.At = result.report, result.at
			if len(result.report.Steps) == 0 {
				s.status = "Apply returned no state results; outcome not verified"
			} else if result.report.Failed > 0 {
				s.status = "Apply returned with failed states; inspect results"
			} else {
				s.status = "Apply returned; inspect results (no automatic repeat)"
			}
		}
	}
	s.offset = min(s.offset, ui.HighstateScrollLimit(m.highstateViewData()))
	return m, nil, true
}

func boundedStateField(text string, size int) string {
	if len(text) <= size {
		return text
	}
	return strings.ToValidUTF8(text[:size], "") + "… (display truncated)"
}

func stateViewReport(report highstate.Report) ui.HighstateReport {
	v := ui.HighstateReport{Proposed: report.Proposed, Changed: report.Changed, Unchanged: report.Unchanged, Failed: report.Failed}
	for _, step := range report.Steps {
		status := "unchanged"
		switch {
		case step.Result == nil:
			status = "proposed"
		case !*step.Result:
			status = "failed"
		case string(step.Changes) != "{}":
			status = "changed"
		}
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, step.Changes, "", "  "); err != nil {
			pretty.Write(step.Changes)
		}
		v.Steps = append(v.Steps, ui.HighstateStep{ID: boundedStateField(step.ID, 256), Status: status,
			Comment: boundedStateField(step.Comment, 2048), Changes: boundedStateField(pretty.String(), 2048)})
	}
	return v
}

func (m Model) highstateViewData() ui.HighstateViewData {
	v := ui.HighstateViewData{Width: m.width, Height: m.height, Context: m.context}
	s := m.highstate
	if s == nil {
		return v
	}
	v.ID, v.Offset, v.Confirm, v.Busy, v.Attempted, v.Help, v.Status = s.id, s.offset, s.confirm, s.busy, s.attempted, s.help, s.status
	v.Presence = "unknown"
	if !m.presence.At.IsZero() && m.presence.Err == nil && !m.presence.Busy {
		v.Presence = "not observed"
		for _, id := range m.presence.Value {
			if id == s.id {
				v.Presence = "connected"
				break
			}
		}
	}
	v.Preview = ui.Source{At: s.preview.At, Err: s.preview.Err, Busy: s.preview.Busy}
	v.Apply = ui.Source{At: s.apply.At, Err: s.apply.Err, Busy: s.apply.Busy}
	v.PreviewReport, v.ApplyReport = stateViewReport(s.preview.Value), stateViewReport(s.apply.Value)
	v.CanApply = m.canApplyHighstate()
	v.Expired = !s.preview.At.IsZero() && time.Since(s.preview.At) > highstatePreviewLifetime
	return v
}

func (m Model) updateHighstate(msg tea.Msg) (tea.Model, tea.Cmd) {
	s := m.highstate
	if s == nil {
		m.activeView = 0
		return m, nil
	}
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	key := keyMsg.String()
	if s.confirm {
		switch key {
		case "y":
			return m, m.applyHighstate()
		case "n", "esc":
			s.confirm = false
			s.status = "Apply canceled; no job sent"
		}
		return m, nil
	}
	if s.help {
		if key == "?" || key == "esc" {
			s.help, s.offset = false, 0
			return m, nil
		}
		if key != "up" && key != "down" && key != "k" && key != "j" && key != "pgup" && key != "pgdown" {
			return m, nil
		}
	}
	switch key {
	case "?":
		if !s.busy {
			s.help, s.offset = !s.help, 0
		}
	case "esc":
		if s.busy {
			s.status = "Wait for CLI result; remote job may continue even if app exits"
			return m, nil
		}
		m.activeView = 0
	case "q":
		m.stopEvents()
		return m, tea.Quit
	case "p":
		return m, m.previewHighstate()
	case "a":
		if m.canApplyHighstate() {
			s.confirm, s.status, s.offset = true, "", 0
		}
	case "up", "down", "k", "j", "pgup", "pgdown":
		s.offset = max(0, min(s.offset+scrollDelta(key, ui.PageRows(m.height, 1, false)), ui.HighstateScrollLimit(m.highstateViewData())))
	}
	return m, nil
}
