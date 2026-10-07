package app

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"saltrtui/internal/metrics"
	"saltrtui/internal/ui"
)

type graphProbe struct {
	context, id string
	generation  int
	kernel      string
	err         error
	capability  metrics.Capability
}

type graphSample struct {
	context, id          string
	generation, sequence int
	at                   time.Time
	cpu                  metrics.Counters
	memory               metrics.Memory
	cpuErr, memErr       error
	load                 metrics.Load
	loadErr              error
	duration             time.Duration
}

type graphTick struct{ generation, sequence int }

type graphState struct {
	open                    bool
	id                      string
	generation, sequence    int
	cancel                  context.CancelFunc
	ctx                     context.Context
	busy, paused, supported bool
	status                  string
	previous                metrics.Counters
	hasPrevious             bool
	points                  []metrics.Point
	last                    time.Time
	help                    bool
	capability              metrics.Capability
	started                 time.Time
	duration                time.Duration
	failures                int
	breakNext               bool
	spacing                 time.Duration
	failed                  bool
}

func (m *Model) stopGraph() {
	if m.graph.cancel != nil {
		m.graph.cancel()
		m.graph.cancel = nil
	}
	m.graph.generation++
	m.graph.busy = false
}

func (m *Model) openGraph() tea.Cmd {
	if m.selected == "" {
		return nil
	}
	m.stopGraph()
	m.graph = graphState{open: true, id: m.selected, generation: m.graph.generation, busy: true, status: "discovering metric source…"}
	if m.metricBackend == nil {
		m.graph.busy = false
		m.graph.status = "resource graph unavailable"
		m.graph.failed = true
		return nil
	}
	m.graph.ctx, m.graph.cancel = context.WithCancel(context.Background())
	backend, ctxID, id, gen, parent := m.metricBackend, m.context, m.graph.id, m.graph.generation, m.graph.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 18*time.Second)
		defer cancel()
		capability, err := backend.DiscoverMetrics(ctx, id)
		return graphProbe{context: ctxID, id: id, generation: gen, kernel: capability.Kernel, err: err, capability: capability}
	}
}

func (m *Model) sampleGraph() tea.Cmd {
	if !m.graph.open || m.graph.busy || m.graph.paused || !m.graph.supported || m.graph.ctx == nil {
		return nil
	}
	m.graph.busy = true
	m.graph.sequence++
	m.graph.status = "sampling…"
	m.graph.started = time.Now()
	backend, ctxID, id, gen, seq, parent := m.metricBackend, m.context, m.graph.id, m.graph.generation, m.graph.sequence, m.graph.ctx
	source := m.graph.capability.Source
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 18*time.Second)
		defer cancel()
		started := time.Now()
		snapshot, err := backend.ReadMetrics(ctx, id, source)
		if err != nil {
			snapshot.CPUErr, snapshot.MemErr, snapshot.LoadErr = err, err, err
		}
		return graphSample{context: ctxID, id: id, generation: gen, sequence: seq, at: time.Now(), cpu: snapshot.CPU, memory: snapshot.Memory, cpuErr: snapshot.CPUErr, memErr: snapshot.MemErr, load: snapshot.Load, loadErr: snapshot.LoadErr, duration: time.Since(started)}
	}
}

func (m *Model) graphResult(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch value := msg.(type) {
	case graphProbe:
		if !m.graph.open || value.context != m.context || value.id != m.graph.id || value.generation != m.graph.generation || !m.graph.busy || m.graph.sequence != 0 {
			return *m, nil, true
		}
		m.graph.busy = false
		if value.err != nil {
			m.graph.status = "Linux probe: " + value.err.Error()
			m.graph.failed = true
			return *m, nil, true
		}
		if value.kernel != "Linux" {
			m.graph.status = fmt.Sprintf("unsupported kernel %q; Linux required", value.kernel)
			m.graph.failed = true
			return *m, nil, true
		}
		m.graph.supported = true
		m.graph.failed = false
		m.graph.capability = value.capability
		return *m, m.sampleGraph(), true
	case graphSample:
		if !m.graph.open || value.context != m.context || value.id != m.graph.id || value.generation != m.graph.generation || value.sequence != m.graph.sequence || !m.graph.busy {
			return *m, nil, true
		}
		m.graph.busy = false
		if !m.graph.last.IsZero() {
			m.graph.spacing = value.at.Sub(m.graph.last)
		}
		m.graph.last = value.at
		m.graph.duration = value.duration
		point := metrics.Point{At: value.at, BreakBefore: m.graph.breakNext}
		m.graph.breakNext = false
		if value.cpuErr == nil {
			if m.graph.hasPrevious {
				var err error
				point.CPU, point.IO, err = metrics.Percent(m.graph.previous, value.cpu)
				point.CPUValid = err == nil
			}
			m.graph.previous, m.graph.hasPrevious = value.cpu, true
		} else {
			m.graph.hasPrevious = false
		}
		if value.memErr == nil {
			var err error
			point.Mem, err = metrics.Used(value.memory)
			point.MemValid = err == nil
			point.Memory = value.memory
		}
		point.Load, point.LoadValid = value.load, value.loadErr == nil
		m.graph.points = metrics.Append(m.graph.points, point)
		m.graph.status = ""
		m.graph.failed = value.cpuErr != nil || value.memErr != nil || value.loadErr != nil
		if value.cpuErr != nil {
			m.graph.status = "CPU: " + value.cpuErr.Error()
		}
		if value.memErr != nil {
			if m.graph.status != "" {
				m.graph.status += " · "
			}
			m.graph.status += "memory: " + value.memErr.Error()
		}
		if value.loadErr != nil {
			if m.graph.status != "" {
				m.graph.status += " · "
			}
			m.graph.status += "load: " + value.loadErr.Error()
		}
		if value.cpuErr != nil && value.memErr != nil && value.loadErr != nil {
			m.graph.failures++
		} else {
			m.graph.failures = 0
		}
		if m.graph.status == "" && !point.CPUValid {
			m.graph.status = "CPU baseline / counter reset; waiting for next interval"
		}
		if !m.graph.paused {
			gen, seq := m.graph.generation, m.graph.sequence
			return *m, tea.Tick(graphDelay(m.graph.started, time.Now(), graphInterval(m.graph.failures)), func(time.Time) tea.Msg { return graphTick{gen, seq} }), true
		}
		return *m, nil, true
	case graphTick:
		if !m.graph.open || value.generation != m.graph.generation || value.sequence != m.graph.sequence {
			return *m, nil, true
		}
		return *m, m.sampleGraph(), true
	}
	return *m, nil, false
}

func (m Model) graphViewData() ui.GraphViewData {
	return ui.GraphViewData{Width: m.width, Height: m.height, ID: m.graph.id, Context: m.context,
		Points: m.graph.points, Busy: m.graph.busy, Paused: m.graph.paused, Help: m.graph.help, Ready: m.graph.supported, Failed: m.graph.failed,
		Status: m.graph.status, Last: m.graph.last, Source: string(m.graph.capability.Source), CPUs: m.graph.capability.CPUs, Duration: m.graph.duration, Interval: graphInterval(m.graph.failures), Spacing: m.graph.spacing}
}

func graphInterval(failures int) time.Duration {
	if failures >= 2 {
		return 10 * time.Second
	}
	if failures == 1 {
		return 5 * time.Second
	}
	return 2 * time.Second
}

// Skip missed slots rather than drifting by request duration or catching up.
func graphDelay(started, now time.Time, interval time.Duration) time.Duration {
	elapsed := max(time.Duration(0), now.Sub(started))
	return interval - elapsed%interval
}

func (m Model) updateGraph(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c", "q":
		m.stopEvents()
		m.stopGraph()
		return m, tea.Quit
	case "esc":
		m.stopGraph()
		m.graph.open = false
	case "?":
		m.graph.help = !m.graph.help
	case "space", " ":
		if !m.graph.supported {
			return m, nil
		}
		if !m.graph.paused {
			m.stopGraph()
			m.graph.paused = true
			m.graph.hasPrevious = false
			m.graph.status = "paused"
			m.graph.breakNext = true
		} else {
			m.graph.ctx, m.graph.cancel = context.WithCancel(context.Background())
			m.graph.paused = false
			return m, m.sampleGraph()
		}
	case "r":
		if !m.graph.busy && m.graph.supported && !m.graph.paused {
			return m, m.sampleGraph()
		}
		if !m.graph.busy && !m.graph.supported {
			return m, m.openGraph()
		}
	}
	return m, nil
}
