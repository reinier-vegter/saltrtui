package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"saltrtui/internal/metrics"
)

type metricGateway struct {
	cpu metrics.Counters
	mem metrics.Memory
}

func (g metricGateway) ProbeKernel(context.Context, string) (string, error)        { return "Linux", nil }
func (g metricGateway) ReadCPU(context.Context, string) (metrics.Counters, error)  { return g.cpu, nil }
func (g metricGateway) ReadMemory(context.Context, string) (metrics.Memory, error) { return g.mem, nil }

func TestGraphLifecycleAndCounterGaps(t *testing.T) {
	m := NewWithMetrics(fakeGateway{}, nil, nil, nil, metricGateway{mem: metrics.Memory{Total: 100, Available: 20}}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 80, Height: 15})
	m = update(m, keysLoaded{"master-a", 0, []string{"web-01", "web-02"}, time.Now(), nil})
	m = press(m, 'g')
	if m.activeView != 5 || m.graph.id != "web-01" || !m.graph.busy {
		t.Fatal("Fleet graph did not start for selected minion")
	}
	gen := m.graph.generation
	m = update(m, graphProbe{"master-a", "web-01", gen, "Linux", nil})
	if !m.graph.supported || !m.graph.busy || m.graph.sequence != 1 {
		t.Fatal("Linux probe did not start first sample")
	}
	a := metrics.Counters{User: 10, Idle: 100, IOWait: 5}
	at := time.Now()
	m = update(m, graphSample{context: "master-a", id: "web-01", generation: gen, sequence: 1, at: at, cpu: a, memory: metrics.Memory{Total: 100, Available: 20}})
	if m.graph.points[0].CPUValid || !m.graph.points[0].MemValid || m.graph.points[0].Mem != 80 {
		t.Fatal("initial CPU must be a gap, while memory may be sampled")
	}
	m = update(m, graphTick{gen, 1})
	if !m.graph.busy || m.graph.sequence != 2 {
		t.Fatal("timer failed to start next non-overlapping cycle")
	}
	m = update(m, graphTick{gen, 1})
	if m.graph.sequence != 2 {
		t.Fatal("old timer started an overlapping cycle")
	}
	b := metrics.Counters{User: 30, Idle: 150, IOWait: 15}
	m = update(m, graphSample{context: "master-a", id: "web-01", generation: gen, sequence: 2, at: at.Add(time.Second), cpu: b, memory: metrics.Memory{Total: 100, Available: 50}})
	if !m.graph.points[1].CPUValid || m.graph.points[1].IO < 12 || m.graph.points[1].Mem != 50 {
		t.Fatalf("bad CPU/mem math: %+v", m.graph.points[1])
	}
	m = press(m, ' ')
	if !m.graph.paused || m.graph.hasPrevious {
		t.Fatal("pause should cancel and reset CPU baseline")
	}
	m = update(m, graphTick{gen, 2})
	if m.graph.busy {
		t.Fatal("stale timer polled during pause")
	}
	m = press(m, ' ')
	if m.graph.paused || !m.graph.busy {
		t.Fatal("resume did not immediately sample")
	}
	m = press(m, tea.KeyEscape)
	if m.activeView != 0 || m.graph.cancel != nil {
		t.Fatal("exit did not cancel polling")
	}
	m = update(m, graphSample{context: "master-a", id: "web-01", generation: m.graph.generation - 1, sequence: m.graph.sequence, at: at})
	if len(m.graph.points) != 2 {
		t.Fatal("late sample was accepted after exit")
	}
	m = press(m, tea.KeyDown)
	m = press(m, 'g')
	if m.graph.id != "web-02" || len(m.graph.points) != 0 {
		t.Fatal("graph leaked previous minion samples")
	}
	newGen := m.graph.generation
	m = update(m, graphProbe{"master-a", "web-01", gen, "Linux", nil})
	if newGen != m.graph.generation || m.graph.supported {
		t.Fatal("old minion probe accepted")
	}
	m = update(m, graphProbe{"master-a", "web-02", newGen, "Windows", nil})
	if !strings.Contains(m.graph.status, "Linux required") || m.graph.busy {
		t.Fatal("unsupported platform did not stop cleanly")
	}
	m = press(m, tea.KeyEscape)
}

func TestGraphPartialFailureAndContextGuard(t *testing.T) {
	m := NewWithMetrics(fakeGateway{}, nil, nil, nil, metricGateway{}, "master-a", true)
	m = update(m, keysLoaded{"master-a", 0, []string{"web-01"}, time.Now(), nil})
	m = press(m, 'g')
	gen := m.graph.generation
	m = update(m, graphProbe{"master-a", "web-01", gen, "Linux", nil})
	m = update(m, graphSample{context: "other", id: "web-01", generation: gen, sequence: 1})
	if !m.graph.busy {
		t.Fatal("wrong context completed sample")
	}
	m = update(m, graphSample{context: "master-a", id: "web-01", generation: gen, sequence: 1, cpuErr: errors.New("timeout"), memory: metrics.Memory{Total: 100, Available: 40}})
	if m.graph.hasPrevious || m.graph.points[0].CPUValid || !m.graph.points[0].MemValid || !strings.Contains(m.graph.status, "timeout") {
		t.Fatal("partial failure not preserved independently")
	}
}
