package ui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"saltrtui/internal/metrics"
)

func TestGraphFrameAndCompactMeasurements(t *testing.T) {
	v := GraphViewData{ID: "web-01", Context: "/etc/salt", Points: []metrics.Point{{At: time.Now(), CPUValid: true, MemValid: true, CPU: 25, IO: 5, Mem: 80}, {At: time.Now()}}, Last: time.Now()}
	for _, width := range []int{120, 36, 5, 1} {
		for _, height := range []int{25, 12, 8, 4, 1} {
			for _, help := range []bool{false, true} {
				v.Width, v.Height, v.Help = width, height, help
				lines := strings.Split(RenderGraph(v), "\n")
				if len(lines) != height {
					t.Fatalf("frame height %dx%d: %d", width, height, len(lines))
				}
				for _, line := range lines {
					if lipgloss.Width(line) != width {
						t.Fatalf("frame width %dx%d: %d %q", width, height, lipgloss.Width(line), ansi.Strip(line))
					}
				}
			}
		}
	}
	v.Width, v.Height, v.Help = 80, 12, false
	short := ansi.Strip(RenderGraph(v))
	for _, label := range []string{"CPU", "I/O", "Mem"} {
		if !strings.Contains(short, label) {
			t.Fatalf("missing compact %s reading: %s", label, short)
		}
	}
	graph, current := graphSeries(v.Points, 10, "cpu")
	if !strings.Contains(graph, "·") || current != "—" {
		t.Fatal("gap should not reuse a stale value")
	}
}
