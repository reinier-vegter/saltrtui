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

func (g metricGateway) DiscoverMetrics(context.Context, string) (metrics.Capability, error) {
	return metrics.Capability{Kernel: "Linux", CPUs: 4, Source: metrics.Status}, nil
}
func (g metricGateway) ReadMetrics(context.Context, string, metrics.Source) (metrics.Snapshot, error) {
	return metrics.Snapshot{CPU: g.cpu, Memory: g.mem, Load: metrics.Load{One: 1, Five: 2, Fifteen: 3}}, nil
}

func TestGraphLifecycleAndCounterGaps(t *testing.T) {
	m := NewWithMetrics(fakeGateway{}, nil, nil, nil, metricGateway{mem: metrics.Memory{Total: 100, Available: 20}}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 80, Height: 15})
	m = update(m, keysLoaded{"master-a", 0, []string{"web-01", "web-02"}, time.Now(), nil})
	m = press(m, 'g')
	if m.activeView != 0 || !m.graph.open || m.graph.id != "web-01" || !m.graph.busy {
		t.Fatal("Fleet graph did not start for selected minion")
	}
	gen := m.graph.generation
	m = update(m, graphProbe{context: "master-a", id: "web-01", generation: gen, kernel: "Linux", capability: metrics.Capability{Kernel: "Linux", Source: metrics.Status}})
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
	if m.graph.open || m.activeView != 0 || m.graph.cancel != nil {
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
	m = update(m, graphProbe{context: "master-a", id: "web-01", generation: gen, kernel: "Linux"})
	if newGen != m.graph.generation || m.graph.supported {
		t.Fatal("old minion probe accepted")
	}
	m = update(m, graphProbe{context: "master-a", id: "web-02", generation: newGen, kernel: "Windows"})
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
	m = update(m, graphProbe{context: "master-a", id: "web-01", generation: gen, kernel: "Linux", capability: metrics.Capability{Source: metrics.Status}})
	m = update(m, graphSample{context: "other", id: "web-01", generation: gen, sequence: 1})
	if !m.graph.busy {
		t.Fatal("wrong context completed sample")
	}
	m = update(m, graphSample{context: "master-a", id: "web-01", generation: gen, sequence: 1, cpuErr: errors.New("timeout"), memory: metrics.Memory{Total: 100, Available: 40}})
	if m.graph.hasPrevious || m.graph.points[0].CPUValid || !m.graph.points[0].MemValid || !strings.Contains(m.graph.status, "timeout") {
		t.Fatal("partial failure not preserved independently")
	}
}

func TestGraphCadenceAndBackoff(t *testing.T) {
	start := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ elapsed, want time.Duration }{
		{500 * time.Millisecond, 1500 * time.Millisecond},
		{2 * time.Second, 2 * time.Second},
		{5500 * time.Millisecond, 500 * time.Millisecond},
	} {
		if got := graphDelay(start, start.Add(tc.elapsed), 2*time.Second); got != tc.want {
			t.Fatalf("elapsed %s: %s, want %s", tc.elapsed, got, tc.want)
		}
	}
	for failures, want := range []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 10 * time.Second} {
		if graphInterval(failures) != want {
			t.Fatal("incorrect backoff")
		}
	}
	m := NewWithMetrics(fakeGateway{}, nil, nil, nil, metricGateway{}, "master-a", true)
	m = update(m, keysLoaded{"master-a", 0, []string{"web-01"}, time.Now(), nil})
	model, cmd := m.Update(tea.KeyPressMsg{Code: 'g'})
	m = model.(Model)
	if cmd == nil {
		t.Fatal("discovery effect missing")
	}
	probe := cmd().(graphProbe)
	m = update(m, probe)
	err := errors.New("offline")
	for failure := 1; failure <= 2; failure++ {
		m = update(m, graphSample{context: m.context, id: m.graph.id, generation: m.graph.generation, sequence: m.graph.sequence, at: start.Add(time.Duration(failure) * time.Second), cpuErr: err, memErr: err, loadErr: err})
		if m.graph.failures != failure {
			t.Fatal("failed poll did not back off")
		}
		m = update(m, graphTick{m.graph.generation, m.graph.sequence})
	}
	m = update(m, graphSample{context: m.context, id: m.graph.id, generation: m.graph.generation, sequence: m.graph.sequence, at: start.Add(3 * time.Second), cpuErr: err, memory: metrics.Memory{Total: 100, Available: 40}, loadErr: err})
	if m.graph.failures != 0 || m.graph.points[len(m.graph.points)-1].CPUValid || !m.graph.points[len(m.graph.points)-1].MemValid {
		t.Fatal("partial recovery not preserved")
	}
}

type cancelMetricGateway struct {
	metricGateway
	started chan struct{}
}

func (g cancelMetricGateway) ReadMetrics(ctx context.Context, _ string, _ metrics.Source) (metrics.Snapshot, error) {
	close(g.started)
	<-ctx.Done()
	return metrics.Snapshot{}, ctx.Err()
}

func TestGraphCancelsInflightSnapshot(t *testing.T) {
	started := make(chan struct{})
	m := NewWithMetrics(fakeGateway{}, nil, nil, nil, cancelMetricGateway{started: started}, "master-a", true)
	m = update(m, keysLoaded{"master-a", 0, []string{"web-01"}, time.Now(), nil})
	model, cmd := m.Update(tea.KeyPressMsg{Code: 'g'})
	m = model.(Model)
	model, cmd = m.Update(cmd())
	m = model.(Model)
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("sample did not start")
	}
	m = press(m, tea.KeyEscape)
	select {
	case reply := <-done:
		m = update(m, reply)
		if m.graph.open || m.graph.busy || len(m.graph.points) != 0 {
			t.Fatal("canceled sample changed overlay")
		}
	case <-time.After(time.Second):
		t.Fatal("sample did not cancel promptly")
	}
}

func TestGraphOverlayCapturesInputAndEscapeRestoresFleet(t *testing.T) {
	m := NewWithMetrics(fakeGateway{}, nil, nil, nil, metricGateway{mem: metrics.Memory{Total: 100, Available: 30}}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = update(m, keysLoaded{"master-a", 0, []string{"web-01", "web-02"}, time.Now(), nil})
	m.focus, m.offset, m.detailOffset = 2, 1, 4
	m.search.SetValue("web")
	before := m.View().Content
	model, cmd := m.Update(tea.KeyPressMsg{Code: 'g'})
	m = model.(Model)
	if m.activeView != 0 || !m.graph.open || cmd == nil {
		t.Fatal("overlay did not keep Fleet active")
	}
	probe := cmd().(graphProbe)
	model, cmd = m.Update(probe)
	m = model.(Model)
	if cmd == nil {
		t.Fatal("first sample effect missing")
	}
	sample := cmd().(graphSample)
	m = update(m, sample)
	if !m.graph.points[0].MemValid || !m.graph.points[0].LoadValid || m.graph.points[0].CPUValid {
		t.Fatal("snapshot normalization failed")
	}
	// Duplicate results and discovery replies cannot start another cycle.
	m = update(m, sample)
	m = update(m, probe)
	if len(m.graph.points) != 1 || m.graph.busy {
		t.Fatal("duplicate reply accepted")
	}
	m = press(m, tea.KeyDown)
	m = press(m, '2')
	m = update(m, tea.PasteMsg{Content: "hidden input"})
	if m.activeView != 0 || m.selected != "web-01" || m.search.Value() != "web" {
		t.Fatal("background received modal input")
	}
	m = press(m, '?')
	parent := m.graph.ctx
	m = press(m, tea.KeyEscape)
	if m.graph.open || parent.Err() == nil || m.graph.cancel != nil {
		t.Fatal("Escape did not close/cancel through help")
	}
	if m.focus != 2 || m.offset != 1 || m.detailOffset != 4 || m.View().Content != before {
		t.Fatal("Fleet context changed")
	}
	m = update(m, graphTick{sample.generation, sample.sequence})
	m = update(m, sample)
	if m.graph.busy || len(m.graph.points) != 1 {
		t.Fatal("closed overlay consumed late work")
	}
}

func TestGraphEscapeDuringDiscoveryAndPauseBreak(t *testing.T) {
	m := NewWithMetrics(fakeGateway{}, nil, nil, nil, metricGateway{}, "master-a", true)
	m = update(m, keysLoaded{"master-a", 0, []string{"web-01"}, time.Now(), nil})
	model, cmd := m.Update(tea.KeyPressMsg{Code: 'g'})
	m = model.(Model)
	parent := m.graph.ctx
	m = press(m, tea.KeyEscape)
	if parent.Err() == nil || m.graph.open {
		t.Fatal("discovery wasn't canceled")
	}
	m = update(m, cmd())
	if m.graph.busy || m.graph.supported {
		t.Fatal("late discovery accepted")
	}
	m = press(m, 'g')
	m = update(m, graphProbe{context: m.context, id: m.graph.id, generation: m.graph.generation, kernel: "Linux", capability: metrics.Capability{Source: metrics.Status}})
	m = update(m, graphSample{context: m.context, id: m.graph.id, generation: m.graph.generation, sequence: m.graph.sequence, at: time.Now(), memory: metrics.Memory{Total: 100, Available: 50}})
	m = press(m, ' ')
	m = press(m, ' ')
	m = update(m, graphSample{context: m.context, id: m.graph.id, generation: m.graph.generation, sequence: m.graph.sequence, at: time.Now(), memory: metrics.Memory{Total: 100, Available: 50}})
	if !m.graph.points[1].BreakBefore || m.graph.points[1].CPUValid {
		t.Fatal("resume connected across pause")
	}
}
