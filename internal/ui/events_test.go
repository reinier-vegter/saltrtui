package ui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestEventsFrameSearchAndPayload(t *testing.T) {
	v := EventsViewData{Context: "/etc/salt", Family: "all", Selected: 0, Inspected: 0,
		Rows: []EventRow{{Tag: "salt/job/123/ret/web-1", Minion: "web-1", JID: "123", Observed: time.Now(), Data: json.RawMessage(`{"return":{"os":"Linux"}}`)}}}
	for _, w := range []int{120, 80, 36, 5, 1} {
		for _, h := range []int{25, 8, 4, 1} {
			for _, focus := range []int{0, 1} {
				for _, help := range []bool{false, true} {
					v.Width, v.Height, v.Focus, v.Help = w, h, focus, help
					lines := strings.Split(RenderEvents(v), "\n")
					if len(lines) != h {
						t.Fatalf("frame height %dx%d: %d", w, h, len(lines))
					}
					for _, line := range lines {
						if lipgloss.Width(line) != w {
							t.Fatalf("frame width %dx%d: %d %q", w, h, lipgloss.Width(line), ansi.Strip(line))
						}
					}
				}
			}
		}
	}
	v.Width, v.Height, v.Help, v.Focus, v.Paused, v.Unseen, v.Dropped = 100, 20, false, 1, true, 3, 4
	view := ansi.Strip(RenderEvents(v))
	if !strings.Contains(view, "os") || !strings.Contains(view, "paused +3") || !strings.Contains(view, "dropped 4") {
		t.Fatalf("payload or gap diagnostics not rendered: %s", view)
	}
	v.Width, v.Height, v.Focus, v.Searching, v.Search = 36, 8, 0, true, "/ salt/job"
	if view := ansi.Strip(RenderEvents(v)); !strings.Contains(view, "salt/job") {
		t.Fatalf("search field not rendered: %s", view)
	}
}
