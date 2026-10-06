package ui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"saltrtui/internal/metrics"
)

func graphFixture() GraphViewData {
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	points := []metrics.Point{}
	for i := 0; i < 30; i++ {
		points = append(points, metrics.Point{At: at.Add(time.Duration(i) * 2 * time.Second), CPUValid: true, MemValid: true, LoadValid: true, CPU: float64(10 + i*2), IO: float64(i % 7), Mem: 80, Memory: metrics.Memory{Total: 8 << 30, Available: 2 << 30}, Load: metrics.Load{One: 1.2, Five: 2.3, Fifteen: 3.4}})
	}
	return GraphViewData{ID: "web-01", Context: "/etc/salt", Points: points, Last: points[len(points)-1].At, Source: "ps", CPUs: 4, Interval: 2 * time.Second}
}

func assertGraphSize(t *testing.T, rendered string, width, height int) {
	t.Helper()
	lines := strings.Split(rendered, "\n")
	if len(lines) != height {
		t.Fatalf("%dx%d: %d lines", width, height, len(lines))
	}
	for _, line := range lines {
		if lipgloss.Width(line) != width {
			t.Fatalf("%dx%d: %d cells: %q", width, height, lipgloss.Width(line), ansi.Strip(line))
		}
	}
}

func TestGraphFrameAndResponsivePlots(t *testing.T) {
	v := graphFixture()
	for _, width := range []int{160, 120, 80, 36, 5, 1} {
		for _, height := range []int{50, 30, 25, 12, 8, 4, 1} {
			for _, help := range []bool{false, true} {
				v.Width, v.Height, v.Help = width, height, help
				assertGraphSize(t, RenderGraph(v), width, height)
			}
		}
	}
	v.Width, v.Height, v.Help = 120, 30, false
	text := ansi.Strip(RenderGraph(v))
	for _, label := range []string{"CPU busy", "I/O wait", "Memory used", "Load average", "esc close", "last " + v.Last.Local().Format("15:04:05"), "1m 1.20", "100"} {
		if !strings.Contains(text, label) {
			t.Fatalf("missing %s: %s", label, text)
		}
	}
	if !strings.Contains(RenderGraph(v), "\x1b[") {
		t.Fatal("missing styles")
	}
	dots := 0
	for _, r := range text {
		if r >= 0x2801 && r <= 0x28ff {
			dots++
		}
	}
	if dots < 20 {
		t.Fatal("missing multi-row plots")
	}
}

func TestGraphOverlayKeepsFleetVisible(t *testing.T) {
	for _, size := range [][2]int{{120, 30}, {80, 24}, {36, 12}, {5, 4}, {1, 1}} {
		v := graphFixture()
		v.Width, v.Height = size[0], size[1]
		fleet := Render(ViewData{Width: v.Width, Height: v.Height, Selected: "web-01"})
		out := RenderGraphOverlay(fleet, v)
		assertGraphSize(t, out, v.Width, v.Height)
		before, after := strings.Split(fleet, "\n"), strings.Split(out, "\n")
		if size[1] >= 12 && ansi.Strip(before[0]) != ansi.Strip(after[0]) {
			t.Fatal("overlay replaced Fleet header")
		}
		if size[0] >= 80 && !strings.Contains(ansi.Strip(out), "Resources · web-01") {
			t.Fatal("overlay missing")
		}
		if size[0] == 80 {
			for _, label := range []string{"CPU busy", "Memory used", "I/O wait", "Load"} {
				if !strings.Contains(ansi.Strip(out), label) {
					t.Fatalf("normal terminal missing %s", label)
				}
			}
		}
	}
	w, h := GraphOverlaySize(120, 30)
	if w != 96 || h != 24 {
		t.Fatalf("overlay size %dx%d", w, h)
	}
}

func TestPlotGapsUseTimeNotSampleIndex(t *testing.T) {
	end := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	v := GraphViewData{Last: end, Interval: 2 * time.Second, Points: []metrics.Point{
		{At: end.Add(-60 * time.Second), CPU: 50, CPUValid: true},
		{At: end.Add(-58 * time.Second)},
		{At: end, CPU: 50, CPUValid: true},
	}}
	lines := plotGraph(v, 0, 67, 9)
	// The minute-long missing interval must not be filled with a line.
	dots := 0
	for _, r := range ansi.Strip(strings.Join(lines, "\n")) {
		if r >= 0x2801 && r <= 0x28ff {
			dots++
		}
	}
	if dots != 2 {
		t.Fatalf("gap interpolated: %d cells", dots)
	}
	v.Points = append(v.Points, metrics.Point{At: end.Add(2 * time.Second), CPU: 80, CPUValid: true})
	v.Last = end.Add(2 * time.Second)
	if !strings.Contains(ansi.Strip(graphHeading(v, 0, 60)), "80.0%") {
		t.Fatal("current reading incorrect")
	}
	v.Points = append(v.Points, metrics.Point{At: end.Add(4 * time.Second)})
	if !strings.Contains(ansi.Strip(graphHeading(v, 0, 60)), "—") {
		t.Fatal("gap reused stale reading")
	}
}
