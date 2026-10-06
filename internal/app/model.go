package app

import (
	"context"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"saltrtui/internal/assignments"
	"saltrtui/internal/cache"
	"saltrtui/internal/console"
	"saltrtui/internal/events"
	"saltrtui/internal/fleet"
	"saltrtui/internal/highstate"
	"saltrtui/internal/jobs"
	"saltrtui/internal/keys"
	"saltrtui/internal/metrics"
	"saltrtui/internal/release"
	"saltrtui/internal/ui"
)

type jobsLoaded struct {
	context string
	request int
	items   []jobs.Summary
	at      time.Time
	err     error
}
type jobLoaded struct {
	context string
	request int
	jid     string
	value   jobs.Detail
	at      time.Time
	err     error
}
type jobDetail struct {
	observation fleet.Observation[jobs.Detail]
	request     int
}

type keysLoaded struct {
	context string
	request int
	ids     []string
	at      time.Time
	err     error
}
type presenceLoaded struct {
	context string
	request int
	ids     []string
	at      time.Time
	err     error
}
type grainsLoaded struct {
	context string
	request int
	id      string
	grains  fleet.Grains
	at      time.Time
	err     error
}

type detail struct {
	observation fleet.Observation[fleet.Grains]
	request     int
}

type Model struct {
	workCtx                                             context.Context
	workCancel                                          context.CancelFunc
	backend                                             fleet.Gateway
	jobBackend                                          jobs.Gateway
	consoleBackend                                      console.Gateway
	eventBackend                                        events.Gateway
	metricBackend                                       metrics.Gateway
	keyBackend                                          keys.Gateway
	highstateBackend                                    highstate.Gateway
	assignmentBackend                                   assignments.Gateway
	assignments                                         *assignmentsState
	assignmentsRequest                                  int
	highstate                                           *highstateState
	highstateRequest                                    int
	key                                                 keyState
	graph                                               graphState
	event                                               eventState
	eventGeneration                                     int
	consoles                                            map[string]*consoleState
	consoleID                                           string
	target                                              *targetState
	savedTarget                                         *savedTarget
	targetRequest                                       int
	activeView                                          int
	jobSearch                                           textinput.Model
	jobFocus, jobOffset, jobDetailOffset, jobHelpOffset int
	selectedJob                                         string
	jobRequest                                          int
	jobList                                             fleet.Observation[[]jobs.Summary]
	jobDetails                                          map[string]jobDetail
	context                                             string
	search                                              textinput.Model
	detailSearch                                        textinput.Model
	width                                               int
	height                                              int
	altScreen                                           bool
	focus                                               int // 0 targets, 1 workspace, 2 details
	help                                                bool
	offset                                              int
	overviewOffset                                      int
	detailOffset                                        int
	helpOffset                                          int
	selected                                            string
	request                                             int
	keys                                                fleet.Observation[[]string]
	presence                                            fleet.Observation[[]string]
	details                                             map[string]detail
	version                                             string
	updateStore                                         *cache.Store
	installation                                        release.Installation
	availableUpdate                                     string
	updateInstalled                                     bool
	updateIndex                                         int
	updatePhase                                         string
	updateText                                          string
	updateGeneration                                    int
	updateCancel                                        func()
	updateDestination                                   string
	updateSudo                                          bool
}

func New(backend fleet.Gateway, configDir string, altScreen bool) Model {
	return NewWithJobs(backend, nil, configDir, altScreen)
}

func NewWithJobs(backend fleet.Gateway, jobBackend jobs.Gateway, configDir string, altScreen bool) Model {
	return NewWithConsole(backend, jobBackend, nil, configDir, altScreen)
}

func NewWithConsole(backend fleet.Gateway, jobBackend jobs.Gateway, consoleBackend console.Gateway, configDir string, altScreen bool) Model {
	return NewWithEvents(backend, jobBackend, consoleBackend, nil, configDir, altScreen)
}

func NewWithEvents(backend fleet.Gateway, jobBackend jobs.Gateway, consoleBackend console.Gateway, eventBackend events.Gateway, configDir string, altScreen bool) Model {
	return NewWithMetrics(backend, jobBackend, consoleBackend, eventBackend, nil, configDir, altScreen)
}

func NewWithMetrics(backend fleet.Gateway, jobBackend jobs.Gateway, consoleBackend console.Gateway, eventBackend events.Gateway, metricBackend metrics.Gateway, configDir string, altScreen bool) Model {
	return NewWithKeys(backend, jobBackend, consoleBackend, eventBackend, metricBackend, nil, configDir, altScreen)
}

func NewWithKeys(backend fleet.Gateway, jobBackend jobs.Gateway, consoleBackend console.Gateway, eventBackend events.Gateway, metricBackend metrics.Gateway, keyBackend keys.Gateway, configDir string, altScreen bool) Model {
	return NewWithHighstate(backend, jobBackend, consoleBackend, eventBackend, metricBackend, keyBackend, nil, configDir, altScreen)
}

func NewWithHighstate(backend fleet.Gateway, jobBackend jobs.Gateway, consoleBackend console.Gateway, eventBackend events.Gateway, metricBackend metrics.Gateway, keyBackend keys.Gateway, highstateBackend highstate.Gateway, configDir string, altScreen bool) Model {
	return NewWithUpdates(backend, jobBackend, consoleBackend, eventBackend, metricBackend, keyBackend, highstateBackend, configDir, altScreen, "dev", nil)
}

// NewWithUpdates is the full constructor: version and updateStore enable the
// background release check and in-app updater. Callers without either value
// get a Model that never advertises or performs an update.
func NewWithUpdates(backend fleet.Gateway, jobBackend jobs.Gateway, consoleBackend console.Gateway, eventBackend events.Gateway, metricBackend metrics.Gateway, keyBackend keys.Gateway, highstateBackend highstate.Gateway, configDir string, altScreen bool, version string, updateStore *cache.Store) Model {
	if configDir == "" {
		configDir = "Salt defaults"
	}
	input := textinput.New()
	input.Prompt = "/ "
	input.Placeholder = "filter minion IDs"
	input.SetWidth(24)
	grainInput := textinput.New()
	grainInput.Prompt = "/ "
	grainInput.Placeholder = "filter grains"
	grainInput.SetWidth(24)
	jobInput := textinput.New()
	jobInput.Prompt = "/ "
	jobInput.Placeholder = "filter jobs"
	jobInput.SetWidth(24)
	assignmentBackend, _ := backend.(assignments.Gateway)
	m := Model{backend: backend, jobBackend: jobBackend, consoleBackend: consoleBackend, eventBackend: eventBackend, metricBackend: metricBackend, keyBackend: keyBackend, highstateBackend: highstateBackend, assignmentBackend: assignmentBackend, key: newKeyState(), event: newEventState(), consoles: make(map[string]*consoleState), jobSearch: jobInput, jobDetails: make(map[string]jobDetail), context: configDir, altScreen: altScreen, search: input, detailSearch: grainInput, details: make(map[string]detail),
		keys: fleet.Observation[[]string]{Busy: true}, presence: fleet.Observation[[]string]{Busy: true},
		version: version, updateStore: updateStore}
	m.workCtx, m.workCancel = context.WithCancel(context.Background())
	if eventBackend != nil {
		m.event.lifetime, m.event.lifetimeCancel = context.WithCancel(context.Background())
		m.event.ctx, m.event.cancel = context.WithCancel(m.event.lifetime)
		m.eventGeneration = 1
	}
	return m
}

func (m *Model) visibleJobs() []jobs.Summary {
	return jobs.Filter(m.jobList.Value, m.jobSearch.Value())
}

func (m *Model) alignJobSelection() {
	items := m.visibleJobs()
	for _, job := range items {
		if job.JID == m.selectedJob {
			return
		}
	}
	m.selectedJob = ""
	if len(items) > 0 {
		m.selectedJob = items[0].JID
	}
	m.jobOffset = 0
	m.jobDetailOffset = 0
}

func (m *Model) moveJob(delta int) {
	items := m.visibleJobs()
	if len(items) == 0 {
		return
	}
	index := 0
	for i, job := range items {
		if job.JID == m.selectedJob {
			index = i
			break
		}
	}
	index = max(0, min(len(items)-1, index+delta))
	if m.selectedJob != items[index].JID {
		m.jobDetailOffset = 0
	}
	m.selectedJob = items[index].JID
	rows := ui.ListRows(m.height)
	if index < m.jobOffset {
		m.jobOffset = index
	} else if index >= m.jobOffset+rows {
		m.jobOffset = index - rows + 1
	}
}

func (m *Model) loadJobs() tea.Cmd {
	if m.jobBackend == nil || m.jobList.Busy {
		return nil
	}
	m.jobRequest++
	m.jobList.Busy = true
	ctxID, req, backend := m.context, m.jobRequest, m.jobBackend
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
		defer cancel()
		items, err := backend.ListRecentJobs(ctx)
		return jobsLoaded{ctxID, req, items, time.Now(), err}
	}
}

func (m *Model) loadJob() tea.Cmd {
	jid := m.selectedJob
	if jid == "" || m.jobBackend == nil {
		return nil
	}
	m.jobFocus = 1
	entry := m.jobDetails[jid]
	if entry.observation.Busy {
		return nil
	}
	entry.request++
	entry.observation.Busy = true
	m.jobDetails[jid] = entry
	ctxID, req, backend := m.context, entry.request, m.jobBackend
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
		defer cancel()
		value, err := backend.ReadJob(ctx, jid)
		return jobLoaded{ctxID, req, jid, value, time.Now(), err}
	}
}

func (m Model) jobsViewData() ui.JobsViewData {
	entry := m.jobDetails[m.selectedJob]
	items := m.visibleJobs()
	rows := make([]ui.JobSummary, 0, len(items))
	for _, job := range items {
		rows = append(rows, ui.JobSummary{JID: job.JID, Function: job.Function, Target: job.Target, TargetType: job.TargetType, StartTime: job.StartTime})
	}
	job := entry.observation.Value
	returns := make(map[string]ui.JobReturn, len(job.Returns))
	for id, value := range job.Returns {
		returns[id] = ui.JobReturn{Value: value.Value, Retcode: value.Retcode, Success: value.Success}
	}
	detail := ui.JobDetail{JobSummary: ui.JobSummary{JID: job.JID, Function: job.Function, Target: job.Target, TargetType: job.TargetType, StartTime: job.StartTime}, Minions: job.Minions, Returns: returns}
	return ui.JobsViewData{Context: m.context, Width: m.width, Height: m.height, Focus: m.jobFocus, Offset: m.jobOffset, DetailOffset: m.jobDetailOffset, HelpOffset: m.jobHelpOffset, Help: m.help, Searching: m.jobSearch.Focused(), Query: m.jobSearch.Value(), Search: m.jobSearch.View(), Selected: m.selectedJob, Items: rows, List: ui.Source{At: m.jobList.At, Err: m.jobList.Err, Busy: m.jobList.Busy}, Detail: ui.Source{At: entry.observation.At, Err: entry.observation.Err, Busy: entry.observation.Busy}, Job: detail}
}

func (m Model) updateJobs(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		if m.jobSearch.Focused() {
			var cmd tea.Cmd
			m.jobSearch, cmd = m.jobSearch.Update(msg)
			m.alignJobSelection()
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
			} else {
				delta := scrollDelta(key, ui.PageRows(m.height, 1, false))
				m.jobHelpOffset = max(0, min(m.jobHelpOffset+delta, ui.JobsScrollLimit(m.jobsViewData(), 2)))
			}
			return m, nil
		}
		if m.jobSearch.Focused() {
			if key == "enter" || key == "esc" {
				m.jobSearch.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.jobSearch, cmd = m.jobSearch.Update(msg)
			m.alignJobSelection()
			return m, cmd
		}
		switch key {
		case "1":
			m.activeView = 0
		case "3":
			return m, m.openAssignments()
		case "4":
			return m, m.openEvents()
		case "6":
			return m, m.openKeys()
		case "q":
			m.stopEvents()
			return m, tea.Quit
		case "?":
			m.help = true
			m.jobHelpOffset = 0
		case "/":
			if m.jobFocus == 0 {
				return m, m.jobSearch.Focus()
			}
		case "tab", "shift+tab":
			m.jobFocus = 1 - m.jobFocus
		case "esc":
			m.jobFocus = 0
		case "r":
			if m.jobFocus == 0 {
				return m, m.loadJobs()
			}
			return m, m.loadJob()
		case "enter":
			return m, m.loadJob()
		case "up", "down", "j", "k", "pgup", "pgdown":
			delta := scrollDelta(key, ui.PageRows(m.height, 1, false))
			if m.jobFocus == 0 {
				m.moveJob(delta)
			} else {
				m.jobDetailOffset = max(0, min(m.jobDetailOffset+delta, ui.JobsScrollLimit(m.jobsViewData(), 1)))
			}
		}
	}
	return m, nil
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refreshCmds(), m.eventStartCmd(), m.updateCheckCmd())
}

// Close releases the long-lived listener even when Run stops without a keypress.
func (m *Model) Close() {
	if m.updateCancel != nil {
		m.updateCancel()
		m.updateCancel = nil
	}
	if m.workCancel != nil {
		m.workCancel()
	}
	if m.event.lifetimeCancel != nil {
		m.event.lifetimeCancel()
	}
	m.stopEvents()
	m.stopGraph()
}

func (m Model) refreshCmds() tea.Cmd {
	request, contextID, backend := m.request, m.context, m.backend
	return tea.Batch(func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
		defer cancel()
		ids, err := backend.ListAcceptedKeys(ctx)
		return keysLoaded{contextID, request, ids, time.Now(), err}
	}, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
		defer cancel()
		ids, err := backend.ReadPresence(ctx)
		return presenceLoaded{contextID, request, ids, time.Now(), err}
	})
}

func (m *Model) refresh() tea.Cmd {
	if m.keys.Busy || m.presence.Busy {
		return nil
	}
	m.request++
	m.keys.Busy, m.presence.Busy = true, true
	return m.refreshCmds()
}

func (m *Model) visibleIDs() []string {
	return fleet.FilterIDs(m.keys.Value, m.search.Value())
}

func (m *Model) alignSelection() {
	previous := m.selected
	ids := m.visibleIDs()
	for _, id := range ids {
		if id == m.selected {
			return
		}
	}
	if len(ids) > 0 {
		m.selected = ids[0]
	} else {
		m.selected = ""
	}
	m.offset = 0
	m.detailOffset = 0
	if previous != m.selected {
		m.detailSearch.SetValue("")
		m.detailSearch.Blur()
	}
}

func (m *Model) move(delta int) {
	ids := m.visibleIDs()
	if len(ids) == 0 {
		return
	}
	index := 0
	for i, id := range ids {
		if id == m.selected {
			index = i
			break
		}
	}
	index = max(0, min(len(ids)-1, index+delta))
	if m.selected != ids[index] {
		m.detailOffset = 0
		m.detailSearch.SetValue("")
		m.detailSearch.Blur()
	}
	m.selected = ids[index]
	rows := ui.ListRows(m.height)
	if index < m.offset {
		m.offset = index
	}
	if index >= m.offset+rows {
		m.offset = index - rows + 1
	}
}

func (m *Model) inspect() tea.Cmd {
	id := m.selected
	if id == "" {
		return nil
	}
	m.focus = 2
	entry := m.details[id]
	if entry.observation.Busy {
		return nil
	}
	entry.request++
	entry.observation.Busy = true
	m.details[id] = entry
	request, contextID, backend := entry.request, m.context, m.backend
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
		defer cancel()
		grains, err := backend.ReadSelectedGrains(ctx, id)
		return grainsLoaded{contextID, request, id, grains, time.Now(), err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if updated, cmd, handled := m.assignmentsResult(msg); handled {
		return updated, cmd
	}
	if updated, cmd, handled := m.updateResult(msg); handled {
		return updated, cmd
	}
	if updated, cmd, handled := m.highstateResult(msg); handled {
		return updated, cmd
	}
	if updated, cmd, handled := m.keysResult(msg); handled {
		return updated, cmd
	}
	if updated, cmd, handled := m.graphResult(msg); handled {
		return updated, cmd
	}
	if updated, cmd, handled := m.eventsResult(msg); handled {
		return updated, cmd
	}
	if updated, cmd, handled := m.consoleResult(msg); handled {
		return updated, cmd
	}
	if updated, cmd, handled := m.targetResult(msg); handled {
		return updated, cmd
	}
	switch msg := msg.(type) {
	case jobsLoaded:
		if msg.context != m.context || msg.request != m.jobRequest {
			return m, nil
		}
		m.jobList.Busy, m.jobList.Err = false, msg.err
		if msg.err == nil {
			items := append([]jobs.Summary(nil), msg.items...)
			jobs.Sort(items)
			m.jobList.Value, m.jobList.At = items, msg.at
			m.alignJobSelection()
		}
		return m, nil
	case jobLoaded:
		entry, ok := m.jobDetails[msg.jid]
		if msg.context != m.context || !ok || msg.request != entry.request {
			return m, nil
		}
		entry.observation.Busy, entry.observation.Err = false, msg.err
		if msg.err == nil {
			entry.observation.Value, entry.observation.At = msg.value, msg.at
		}
		m.jobDetails[msg.jid] = entry
		if m.selectedJob == msg.jid {
			m.jobDetailOffset = min(m.jobDetailOffset, ui.JobsScrollLimit(m.jobsViewData(), 1))
		}
		return m, nil
	case keysLoaded:
		if msg.context != m.context || msg.request != m.request {
			return m, nil
		}
		m.keys.Busy, m.keys.Err = false, msg.err
		if msg.err == nil {
			m.keys.Value, m.keys.At = fleet.SortUnique(msg.ids), msg.at
			m.alignSelection()
			if m.highstate != nil && m.activeView == 7 && m.highstate.id != m.selected {
				m.highstate.confirm = false
				m.highstate.status = "Fleet selection changed; return to Fleet and review the new minion"
			}
			if m.assignments != nil && m.activeView == 9 && m.assignments.observation.At.IsZero() && !m.assignments.observation.Busy {
				return m, m.loadAssignments()
			}
		}
	case presenceLoaded:
		if msg.context != m.context || msg.request != m.request {
			return m, nil
		}
		m.presence.Busy, m.presence.Err = false, msg.err
		if msg.err == nil {
			m.presence.Value, m.presence.At = fleet.SortUnique(msg.ids), msg.at
		}
	case grainsLoaded:
		entry, ok := m.details[msg.id]
		if msg.context != m.context || !ok || msg.request != entry.request {
			return m, nil
		}
		entry.observation.Busy, entry.observation.Err = false, msg.err
		if msg.err == nil {
			entry.observation.Value, entry.observation.At = msg.grains, msg.at
		}
		m.details[msg.id] = entry
		if msg.id == m.selected {
			m.clampScroll()
		}
	case tea.WindowSizeMsg:
		m.width, m.height = max(0, msg.Width), max(0, msg.Height)
		m.search.SetWidth(max(1, min(32, ui.TargetWidth(m.width)-4)))
		detailWidth := m.width
		if m.width >= 100 {
			left := ui.TargetWidth(m.width)
			middle := (m.width - 4 - left) / 2
			detailWidth = m.width - 4 - left - middle
		} else if m.width >= 68 {
			detailWidth = m.width - 2 - ui.TargetWidth(m.width)
		}
		m.detailSearch.SetWidth(max(1, min(32, detailWidth-4)))
		m.clampScroll()
		m.jobSearch.SetWidth(max(1, min(32, ui.TargetWidth(m.width)-4)))
		m.jobDetailOffset = min(m.jobDetailOffset, ui.JobsScrollLimit(m.jobsViewData(), 1))
		m.jobHelpOffset = min(m.jobHelpOffset, ui.JobsScrollLimit(m.jobsViewData(), 2))
		for id, state := range m.consoles {
			state.input.SetWidth(max(1, m.width-8))
			state.offset = min(state.offset, ui.ConsoleScrollLimit(m.consoleViewData(id)))
		}
		if m.target != nil {
			m.target.input.SetWidth(max(1, m.width-12))
			m.target.offset = min(m.target.offset, ui.TargetScrollLimit(m.targetViewData()))
		}
		m.event.search.SetWidth(max(1, min(32, ui.TargetWidth(m.width)-4)))
		m.event.offset = min(m.event.offset, max(0, len(m.eventIndices())-ui.ListRows(m.height)))
		m.event.detailOffset = min(m.event.detailOffset, ui.EventsScrollLimit(m.eventsViewData(), 1))
		m.event.helpOffset = min(m.event.helpOffset, ui.EventsScrollLimit(m.eventsViewData(), 2))
		m.key.search.SetWidth(max(1, min(32, ui.TargetWidth(m.width)-4)))
		m.key.offset = min(m.key.offset, max(0, len(m.visibleKeys())-ui.ListRows(m.height)))
		m.key.detailOffset = min(m.key.detailOffset, ui.KeysScrollLimit(m.keysViewData(), 1))
		m.key.helpOffset = min(m.key.helpOffset, ui.KeysScrollLimit(m.keysViewData(), 2))
		if m.highstate != nil {
			m.highstate.offset = min(m.highstate.offset, ui.HighstateScrollLimit(m.highstateViewData()))
		}
		if m.assignments != nil {
			m.assignments.offset = min(m.assignments.offset, ui.AssignmentsScrollLimit(m.assignmentsViewData(), 0))
			m.assignments.detailOffset = min(m.assignments.detailOffset, ui.AssignmentsScrollLimit(m.assignmentsViewData(), 1))
			m.assignments.filter.SetWidth(max(1, min(32, ui.TargetWidth(m.width)-4)))
		}
	case tea.PasteMsg:
		if m.graph.open {
			return m, nil
		}
		if m.activeView == 9 {
			return m.updateAssignments(msg)
		}
		if m.activeView == 6 {
			return m.updateKeys(msg)
		}
		if m.activeView == 4 {
			return m.updateEvents(msg)
		}
		if m.activeView == 3 {
			return m.updateTarget(msg)
		}
		if m.activeView == 2 {
			return m.updateConsole(msg)
		}
		if m.activeView == 1 {
			return m.updateJobs(msg)
		}
		if m.search.Focused() {
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			m.alignSelection()
			return m, cmd
		}
		if m.detailSearch.Focused() {
			var cmd tea.Cmd
			m.detailSearch, cmd = m.detailSearch.Update(msg)
			m.detailOffset = 0
			return m, cmd
		}
	case tea.KeyPressMsg:
		key := msg.String()
		if m.activeView == 9 {
			return m.updateAssignments(msg)
		}
		if m.activeView == 8 {
			return m.updateSelfUpdate(msg)
		}
		if key == "ctrl+c" {
			m.stopEvents()
			m.stopGraph()
			return m, tea.Quit
		}
		if m.activeView == 7 {
			return m.updateHighstate(msg)
		}
		if m.activeView == 6 {
			return m.updateKeys(msg)
		}
		if m.graph.open {
			return m.updateGraph(msg)
		}
		if m.activeView == 4 {
			return m.updateEvents(msg)
		}
		if m.activeView == 3 {
			return m.updateTarget(msg)
		}
		if m.activeView == 2 {
			return m.updateConsole(msg)
		}
		if m.activeView == 1 {
			return m.updateJobs(msg)
		}
		if m.help {
			if key == "esc" || key == "?" || key == "q" {
				m.help = false
			} else {
				delta := scrollDelta(key, ui.PageRows(m.height, 3, false))
				m.helpOffset = max(0, min(m.helpOffset+delta, ui.ScrollLimit(m.viewData(), 3)))
			}
			return m, nil
		}
		if m.search.Focused() {
			switch key {
			case "esc", "enter":
				m.search.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			m.alignSelection()
			return m, cmd
		}
		if m.detailSearch.Focused() {
			switch key {
			case "esc", "enter":
				m.detailSearch.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.detailSearch, cmd = m.detailSearch.Update(msg)
			m.detailOffset = 0
			return m, cmd
		}
		if key == "2" {
			m.activeView = 1
			if m.jobList.At.IsZero() && m.jobList.Err == nil {
				return m, m.loadJobs()
			}
			return m, nil
		}
		if key == "3" && !m.help {
			return m, m.openAssignments()
		}
		if key == "4" && !m.help {
			return m, m.openEvents()
		}
		if key == "6" && !m.help {
			return m, m.openKeys()
		}
		if key == "U" && !m.help && m.availableUpdate != "" {
			return m, m.openSelfUpdate()
		}
		switch key {
		case "q":
			m.stopEvents()
			return m, tea.Quit
		case "?":
			m.help = true
			m.helpOffset = 0
		case "/":
			switch m.focus {
			case 0:
				return m, m.search.Focus()
			case 2:
				if m.selected != "" {
					return m, m.detailSearch.Focus()
				}
			}
		case "r":
			cmd := m.refresh()
			return m, cmd
		case "tab":
			m.focus = (m.focus + 1) % 3
		case "shift+tab":
			m.focus = (m.focus + 2) % 3
		case "esc":
			m.focus = 0
		case "up", "k", "down", "j", "pgup", "pgdown":
			delta := scrollDelta(key, ui.PageRows(m.height, m.focus, m.selected != ""))
			switch m.focus {
			case 0:
				m.move(delta)
			case 1:
				m.overviewOffset = max(0, min(m.overviewOffset+delta, ui.ScrollLimit(m.viewData(), 1)))
			case 2:
				m.detailOffset = max(0, min(m.detailOffset+delta, ui.ScrollLimit(m.viewData(), 2)))
			}
		case "enter":
			if m.focus == 0 || m.focus == 2 {
				cmd := m.inspect()
				return m, cmd
			}
		case "s":
			return m, m.openConsole()
		case "t":
			m.openTarget()
		case "g":
			return m, m.openGraph()
		case "h":
			return m, m.openHighstate()
		}
	}
	return m, nil
}

func scrollDelta(key string, page int) int {
	switch key {
	case "up", "k":
		return -1
	case "down", "j":
		return 1
	case "pgup":
		return -page
	case "pgdown":
		return page
	}
	return 0
}

func (m *Model) clampScroll() {
	data := m.viewData()
	m.overviewOffset = min(m.overviewOffset, ui.ScrollLimit(data, 1))
	m.detailOffset = min(m.detailOffset, ui.ScrollLimit(data, 2))
	m.helpOffset = min(m.helpOffset, ui.ScrollLimit(data, 3))
}

func (m Model) viewData() ui.ViewData {
	ids := m.visibleIDs()
	connected := make(map[string]bool, len(m.presence.Value))
	for _, id := range m.presence.Value {
		connected[id] = true
	}
	rows := make([]ui.Row, 0, len(ids))
	for _, id := range ids {
		status := "unknown"
		if !m.presence.At.IsZero() && m.presence.Err == nil && !m.presence.Busy {
			status = "not observed"
			if connected[id] {
				status = "connected"
			}
		}
		rows = append(rows, ui.Row{ID: id, Status: status, Selected: id == m.selected})
	}
	entry := m.details[m.selected]
	data := ui.ViewData{
		Context: m.context, Width: m.width, Height: m.height,
		Focus: m.focus, Help: m.help, Search: m.search.View(), Query: m.search.Value(),
		DetailSearch: m.detailSearch.View(), DetailQuery: m.detailSearch.Value(), DetailSearching: m.detailSearch.Focused(),
		Searching: m.search.Focused(), Rows: rows, Offset: m.offset,
		OverviewOffset: m.overviewOffset, DetailOffset: m.detailOffset, HelpOffset: m.helpOffset,
		Inventory: ui.Source{Count: len(m.keys.Value), At: m.keys.At, Err: m.keys.Err, Busy: m.keys.Busy},
		Presence:  ui.Source{At: m.presence.At, Err: m.presence.Err, Busy: m.presence.Busy},
		Selected:  m.selected, Grains: entry.observation.Value,
		Detail:         ui.Source{At: entry.observation.At, Err: entry.observation.Err, Busy: entry.observation.Busy},
		RunningVersion: m.version, AvailableUpdate: m.availableUpdate,
	}
	if !m.presence.At.IsZero() && m.presence.Err == nil && !m.presence.Busy {
		for _, id := range m.keys.Value {
			if connected[id] {
				data.Connected++
			} else {
				data.NotObserved++
			}
		}
	}
	data.SelectedStatus = "unknown"
	for _, row := range rows {
		if row.ID == m.selected {
			data.SelectedStatus = row.Status
		}
	}
	// Detail data is only shown for the selected minion, never the last
	// asynchronous return from a previously highlighted row.
	return data
}

func (m Model) View() tea.View {
	var content string
	if m.activeView == 9 {
		content = ui.RenderAssignments(m.assignmentsViewData())
	} else if m.activeView == 8 {
		content = ui.RenderSelfUpdate(m.updateViewData())
	} else if m.activeView == 7 {
		content = ui.RenderHighstate(m.highstateViewData())
	} else if m.activeView == 6 {
		content = ui.RenderKeys(m.keysViewData())
	} else if m.activeView == 4 {
		content = ui.RenderEvents(m.eventsViewData())
	} else if m.activeView == 3 {
		content = ui.RenderTarget(m.targetViewData())
	} else if m.activeView == 1 {
		content = ui.RenderJobs(m.jobsViewData())
	} else if m.activeView == 2 {
		content = ui.RenderConsole(m.consoleViewData(m.consoleID))
	} else {
		content = ui.Render(m.viewData())
	}
	if m.graph.open {
		content = ui.RenderGraphOverlay(content, m.graphViewData())
	}
	view := tea.NewView(content)
	view.AltScreen = m.altScreen
	return view
}
