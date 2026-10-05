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
}

type graphSample struct {
	context, id          string
	generation, sequence int
	at                   time.Time
	cpu                  metrics.Counters
	memory               metrics.Memory
	cpuErr, memErr       error
}

type graphTick struct{ generation, sequence int }

type graphState struct {
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
	m.activeView = 5
	m.graph = graphState{id: m.selected, generation: m.graph.generation, busy: true, status: "checking Linux minion…"}
	if m.metricBackend == nil {
		m.graph.busy = false
		m.graph.status = "resource graph unavailable"
		return nil
	}
	m.graph.ctx, m.graph.cancel = context.WithCancel(context.Background())
	backend, ctxID, id, gen, parent := m.metricBackend, m.context, m.graph.id, m.graph.generation, m.graph.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 18*time.Second)
		defer cancel()
		kernel, err := backend.ProbeKernel(ctx, id)
		return graphProbe{ctxID, id, gen, kernel, err}
	}
}

func (m *Model) sampleGraph() tea.Cmd {
	if m.graph.busy || m.graph.paused || !m.graph.supported || m.graph.ctx == nil {
		return nil
	}
	m.graph.busy = true
	m.graph.sequence++
	m.graph.status = "sampling…"
	backend, ctxID, id, gen, seq, parent := m.metricBackend, m.context, m.graph.id, m.graph.generation, m.graph.sequence, m.graph.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 18*time.Second)
		defer cancel()
		cpu, cpuErr := backend.ReadCPU(ctx, id)
		var memory metrics.Memory
		var memErr error
		if ctx.Err() != nil {
			memErr = ctx.Err()
		} else {
			memory, memErr = backend.ReadMemory(ctx, id)
		}
		return graphSample{context: ctxID, id: id, generation: gen, sequence: seq, at: time.Now(), cpu: cpu, memory: memory, cpuErr: cpuErr, memErr: memErr}
	}
}

func (m *Model) graphResult(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch value := msg.(type) {
	case graphProbe:
		if m.activeView != 5 || value.context != m.context || value.id != m.graph.id || value.generation != m.graph.generation {
			return *m, nil, true
		}
		m.graph.busy = false
		if value.err != nil {
			m.graph.status = "Linux probe: " + value.err.Error()
			return *m, nil, true
		}
		if value.kernel != "Linux" {
			m.graph.status = fmt.Sprintf("unsupported kernel %q; Linux required", value.kernel)
			return *m, nil, true
		}
		m.graph.supported = true
		return *m, m.sampleGraph(), true
	case graphSample:
		if m.activeView != 5 || value.context != m.context || value.id != m.graph.id || value.generation != m.graph.generation || value.sequence != m.graph.sequence {
			return *m, nil, true
		}
		m.graph.busy = false
		m.graph.last = value.at
		point := metrics.Point{At: value.at}
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
		}
		m.graph.points = metrics.Append(m.graph.points, point)
		m.graph.status = ""
		if value.cpuErr != nil {
			m.graph.status = "CPU: " + value.cpuErr.Error()
		}
		if value.memErr != nil {
			if m.graph.status != "" {
				m.graph.status += " · "
			}
			m.graph.status += "memory: " + value.memErr.Error()
		}
		if m.graph.status == "" && !point.CPUValid {
			m.graph.status = "CPU baseline / counter reset; waiting for next interval"
		}
		if !m.graph.paused {
			gen, seq := m.graph.generation, m.graph.sequence
			return *m, tea.Tick(10*time.Second, func(time.Time) tea.Msg { return graphTick{gen, seq} }), true
		}
		return *m, nil, true
	case graphTick:
		if m.activeView != 5 || value.generation != m.graph.generation || value.sequence != m.graph.sequence {
			return *m, nil, true
		}
		return *m, m.sampleGraph(), true
	}
	return *m, nil, false
}

func (m Model) graphViewData() ui.GraphViewData {
	return ui.GraphViewData{Width: m.width, Height: m.height, ID: m.graph.id, Context: m.context,
		Points: m.graph.points, Busy: m.graph.busy, Paused: m.graph.paused, Help: m.graph.help,
		Status: m.graph.status, Last: m.graph.last}
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
	case "esc", "1":
		if m.graph.help && key.String() == "esc" {
			m.graph.help = false
			return m, nil
		}
		m.stopGraph()
		m.activeView = 0
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
