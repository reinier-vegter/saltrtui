package ui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestJobsFullscreenAndReturns(t *testing.T) {
	jid := "20260925124000000000"
	data := JobsViewData{Width: 100, Height: 11, Context: "/etc/salt", Selected: jid, Items: []JobSummary{{JID: jid, Function: "test.ping", Target: "web-*"}}, Job: JobDetail{JobSummary: JobSummary{JID: jid, Function: "test.ping", Target: "web-*", StartTime: "now"}, Minions: []string{"a", "b"}, Returns: map[string]JobReturn{"a": {Value: json.RawMessage(`false`)}}}, Detail: Source{At: time.Now()}}
	text := strings.Join(jobDetailLines(data, 30), "\n")
	if !strings.Contains(text, "b: no return recorded") || !strings.Contains(text, "false") {
		t.Fatalf("returns not rendered clearly: %s", text)
	}
	for _, w := range []int{120, 80, 36, 5, 1} {
		for _, h := range []int{25, 8, 4, 1} {
			for _, focus := range []int{0, 1} {
				data.Width, data.Height, data.Focus = w, h, focus
				for _, help := range []bool{false, true} {
					data.Help = help
					lines := strings.Split(RenderJobs(data), "\n")
					if len(lines) != h {
						t.Fatalf("jobs frame height %dx%d: %d", w, h, len(lines))
					}
					for _, line := range lines {
						if lipgloss.Width(line) != w {
							t.Fatalf("jobs frame width %dx%d: %d %q", w, h, lipgloss.Width(line), ansi.Strip(line))
						}
					}
				}
			}
		}
	}
	data.Width, data.Height, data.Help, data.Focus = 36, 8, false, 1
	data.DetailOffset = JobsScrollLimit(data, 1)
	if view := ansi.Strip(RenderJobs(data)); !strings.Contains(view, "Job detail") || !strings.Contains(view, "1 Fleet") {
		t.Fatalf("detail/footer lost: %s", view)
	}
}
