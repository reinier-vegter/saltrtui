package app

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"saltrtui/internal/assignments"
	"saltrtui/internal/fleet"
	"saltrtui/internal/ui"
)

type stateTopLoaded struct {
	context string
	request int
	scope   []string
	top     assignments.Top
	at      time.Time
	err     error
}

type assignmentsState struct {
	request                     int
	scope                       []string
	observation                 fleet.Observation[assignments.Top]
	selected                    string
	expanded                    map[string]bool
	filter                      textinput.Model
	focus, offset, detailOffset int
	help                        bool
}

func newAssignmentsState() *assignmentsState {
	filter := textinput.New()
	filter.Prompt = "/ "
	filter.Placeholder = "filter assignments or minions"
	filter.SetWidth(28)
	return &assignmentsState{expanded: make(map[string]bool), filter: filter}
}

func (m *Model) openAssignments() tea.Cmd {
	if m.assignments == nil {
		m.assignments = newAssignmentsState()
	}
	m.activeView = 9
	if m.assignments.observation.At.IsZero() && !m.assignments.observation.Busy {
		return m.loadAssignments()
	}
	return nil
}

func (m *Model) loadAssignments() tea.Cmd {
	s := m.assignments
	if s == nil || s.observation.Busy || m.assignmentBackend == nil || m.keys.At.IsZero() || m.keys.Busy || m.keys.Err != nil {
		return nil
	}
	m.assignmentsRequest++
	s.request = m.assignmentsRequest
	s.scope = append([]string(nil), m.keys.Value...)
	s.observation.Busy, s.observation.Err = true, nil
	contextID, request, scope, backend := m.context, s.request, append([]string(nil), s.scope...), m.assignmentBackend
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		top, err := backend.ReadStateTop(ctx, scope)
		return stateTopLoaded{contextID, request, scope, top, time.Now(), err}
	}
}

func (m Model) assignmentNodes() []assignments.Node {
	if m.assignments == nil {
		return nil
	}
	return assignments.Tree(m.assignments.observation.Value)
}

func nodeMatches(node assignments.Node, query string) bool {
	return strings.Contains(strings.ToLower(node.Environment+" "+node.State+" "+node.Minion), query)
}

func (m Model) visibleAssignmentNodes() []assignments.Node {
	s := m.assignments
	if s == nil {
		return nil
	}
	nodes := m.assignmentNodes()
	query := strings.ToLower(strings.TrimSpace(s.filter.Value()))
	byID := make(map[string]assignments.Node, len(nodes))
	matching := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
		if query != "" && nodeMatches(node, query) {
			matching[node.ID] = true
		}
	}
	if query != "" {
		for id := range matching {
			for parent := byID[id].ParentID; parent != ""; parent = byID[parent].ParentID {
				matching[parent] = true
			}
		}
	}
	visible := make([]assignments.Node, 0, len(nodes))
	for _, node := range nodes {
		if query != "" {
			if matching[node.ID] {
				visible = append(visible, node)
			}
			continue
		}
		if node.ParentID == "" || s.expanded[node.ParentID] {
			visible = append(visible, node)
		}
	}
	return visible
}

func (m *Model) alignAssignmentSelection() {
	s := m.assignments
	if s == nil {
		return
	}
	for _, node := range m.visibleAssignmentNodes() {
		if node.ID == s.selected {
			return
		}
	}
	s.selected, s.offset, s.detailOffset = "", 0, 0
	if nodes := m.visibleAssignmentNodes(); len(nodes) > 0 {
		s.selected = nodes[0].ID
	}
}

func (m *Model) defaultAssignmentExpansion() {
	if m.assignments == nil {
		return
	}
	for _, node := range m.assignmentNodes() {
		if node.Kind == assignments.EnvironmentNode {
			if _, known := m.assignments.expanded[node.ID]; !known {
				m.assignments.expanded[node.ID] = true
			}
		}
	}
}

func (m *Model) moveAssignment(delta int) {
	s := m.assignments
	if s == nil {
		return
	}
	nodes := m.visibleAssignmentNodes()
	if len(nodes) == 0 {
		return
	}
	index := 0
	for i, node := range nodes {
		if node.ID == s.selected {
			index = i
			break
		}
	}
	index = max(0, min(len(nodes)-1, index+delta))
	if s.selected != nodes[index].ID {
		s.detailOffset = 0
	}
	s.selected = nodes[index].ID
	rows := ui.ListRows(m.height)
	if index < s.offset {
		s.offset = index
	} else if index >= s.offset+rows {
		s.offset = index - rows + 1
	}
}

func (m *Model) expandAssignment() {
	s := m.assignments
	if s == nil {
		return
	}
	for _, node := range m.visibleAssignmentNodes() {
		if node.ID == s.selected && node.Kind != assignments.MinionNode {
			s.expanded[node.ID] = true
			return
		}
	}
}

func (m *Model) collapseAssignment() {
	s := m.assignments
	if s == nil {
		return
	}
	for _, node := range m.assignmentNodes() {
		if node.ID != s.selected {
			continue
		}
		if node.Kind != assignments.MinionNode && s.expanded[node.ID] {
			s.expanded[node.ID] = false
			m.alignAssignmentSelection()
			return
		}
		if node.ParentID != "" {
			s.selected, s.detailOffset = node.ParentID, 0
		}
		return
	}
}

func (m Model) assignmentsResult(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	result, ok := msg.(stateTopLoaded)
	if !ok {
		return m, nil, false
	}
	s := m.assignments
	if s == nil || result.context != m.context || result.request != s.request || !s.observation.Busy {
		return m, nil, true
	}
	s.observation.Busy, s.observation.Err = false, result.err
	if result.err == nil {
		s.observation.Value, s.observation.At = result.top, result.at
		s.scope = append([]string(nil), result.scope...)
		m.defaultAssignmentExpansion()
		m.alignAssignmentSelection()
	}
	return m, nil, true
}

func (m Model) assignmentsViewData() ui.AssignmentsViewData {
	v := ui.AssignmentsViewData{Width: m.width, Height: m.height, Context: m.context}
	s := m.assignments
	if s == nil {
		return v
	}
	for _, node := range m.visibleAssignmentNodes() {
		row := ui.AssignmentNode{ID: node.ID, Environment: node.Environment, State: node.State, Minion: node.Minion, Selected: node.ID == s.selected}
		switch node.Kind {
		case assignments.EnvironmentNode:
			row.Kind, row.Expanded = ui.AssignmentEnvironment, s.expanded[node.ID]
		case assignments.StateNode:
			row.Kind, row.Expanded = ui.AssignmentState, s.expanded[node.ID]
		default:
			row.Kind = ui.AssignmentMinion
		}
		v.Nodes = append(v.Nodes, row)
	}
	v.Scope, v.Selected, v.Focus, v.Offset, v.DetailOffset, v.Help = append([]string(nil), s.scope...), s.selected, s.focus, s.offset, s.detailOffset, s.help
	v.Filter, v.Filtering = s.filter.View(), s.filter.Focused()
	v.Query = s.filter.Value()
	v.Result = ui.Source{Count: len(s.observation.Value), At: s.observation.At, Err: s.observation.Err, Busy: s.observation.Busy}
	v.Available = m.assignmentBackend != nil
	return v
}

func (m Model) updateAssignments(msg tea.Msg) (tea.Model, tea.Cmd) {
	s := m.assignments
	if s == nil {
		m.activeView = 0
		return m, nil
	}
	if paste, ok := msg.(tea.PasteMsg); ok && s.filter.Focused() {
		var cmd tea.Cmd
		s.filter, cmd = s.filter.Update(paste)
		m.alignAssignmentSelection()
		return m, cmd
	}
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	key := keyMsg.String()
	if s.help {
		switch key {
		case "?", "esc":
			s.help = false
		case "q":
			m.stopEvents()
			return m, tea.Quit
		}
		return m, nil
	}
	if s.filter.Focused() {
		if key == "esc" || key == "enter" {
			s.filter.Blur()
			if key == "esc" {
				s.filter.SetValue("")
				m.alignAssignmentSelection()
			}
			return m, nil
		}
		var cmd tea.Cmd
		s.filter, cmd = s.filter.Update(keyMsg)
		m.alignAssignmentSelection()
		return m, cmd
	}
	if cmd, handled := m.switchWorkspace(key); handled {
		return m, cmd
	}
	switch key {
	case "q":
		m.stopEvents()
		return m, tea.Quit
	case "?":
		s.help = true
	case "/":
		return m, s.filter.Focus()
	case "esc":
		s.focus = 0
	case "r":
		return m, m.loadAssignments()
	case "tab", "shift+tab":
		s.focus = 1 - s.focus
	case "enter", "space", "right", "l":
		if s.focus == 0 {
			m.expandAssignment()
		}
	case "left", "h":
		if s.focus == 0 {
			m.collapseAssignment()
		}
	case "up", "down", "pgup", "pgdown":
		delta := scrollDelta(key, ui.PageRows(m.height, s.focus, false))
		if s.focus == 0 {
			m.moveAssignment(delta)
		} else {
			s.detailOffset = max(0, min(s.detailOffset+delta, ui.AssignmentsScrollLimit(m.assignmentsViewData(), 1)))
		}
	}
	return m, nil
}
