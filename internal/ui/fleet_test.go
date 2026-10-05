package ui

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestNarrowFleetKeepsSelectedRowVisible(t *testing.T) {
	data := ViewData{Width: 36, Height: 8, Focus: 0, Context: "/etc/salt",
		Search: "\x1b[31m/ web\x1b[0m", Offset: 0, Rows: []Row{
			{ID: "web-01"}, {ID: "web-02"}, {ID: "web-03"}, {ID: "web-04", Selected: true},
		}}
	view := ansi.Strip(Render(data))
	if !strings.Contains(view, "web-04") || strings.Contains(view, "[31m") {
		t.Fatalf("selected row or search rendered incorrectly:\n%s", view)
	}
	if lines := strings.Count(view, "\n"); lines > data.Height {
		t.Fatalf("render exceeds terminal height: %d > %d", lines, data.Height)
	}
	data.Focus = 1
	if view := ansi.Strip(Render(data)); !strings.Contains(view, "/etc/salt") {
		t.Fatal("narrow layout does not expose the selected config in the overview")
	}
}

func TestFailureAndLoadingKeepViewport(t *testing.T) {
	data := ViewData{Width: 80, Height: 10, Focus: 0, Inventory: Source{Busy: true}}
	for _, want := range []string{"Loading accepted keys", "Keys: denied", "No accepted keys · 6 Keys"} {
		switch want {
		case "Keys: denied":
			data.Inventory = Source{Err: errors.New("denied")}
		case "No accepted keys · 6 Keys":
			data.Inventory = Source{At: time.Now()}
		}
		view := ansi.Strip(Render(data))
		if !strings.Contains(view, want) || len(strings.Split(view, "\n")) != data.Height {
			t.Fatalf("%q state did not render in fullscreen:\n%s", want, view)
		}
	}
}

func TestFleetFillsViewportAndFitsDisplayCells(t *testing.T) {
	data := ViewData{Context: "/etc/salt", Focus: 0, Search: "/ 搜", Rows: []Row{
		{ID: "web-01", Selected: true, Status: "connected"}, {ID: "東京-node", Status: "unknown"},
	}}
	for _, width := range []int{120, 100, 84, 68, 36, 20, 5, 1} {
		for _, height := range []int{25, 8, 5, 4, 3, 2, 1} {
			for _, focus := range []int{0, 1, 2} {
				data.Width, data.Height, data.Focus = width, height, focus
				for _, help := range []bool{false, true} {
					data.Help = help
					view := Render(data)
					lines := strings.Split(view, "\n")
					if len(lines) != height {
						t.Fatalf("%dx%d focus=%d help=%v: got %d rows:\n%s", width, height, focus, help, len(lines), ansi.Strip(view))
					}
					for row, line := range lines {
						if cells := lipgloss.Width(line); cells != width {
							t.Fatalf("%dx%d focus=%d help=%v row=%d: got %d cells: %q", width, height, focus, help, row, cells, ansi.Strip(line))
						}
					}
				}
			}
		}
	}
}

func TestScrollablePanelsKeepFooterAndRevealLowerContent(t *testing.T) {
	data := ViewData{Width: 112, Height: 8, Focus: 2, Selected: "web-01",
		Rows: []Row{{ID: "web-01", Selected: true}}, Grains: map[string]any{"kernel": "Linux"}, Detail: Source{At: time.Now()}, Search: "/ ",
	}
	initial := ansi.Strip(Render(data))
	if strings.Contains(initial, "kernel: Linux") || !strings.Contains(initial, "ID: web-01") {
		t.Fatalf("details should start at the top:\n%s", initial)
	}
	data.DetailOffset = 99
	bottom := ansi.Strip(Render(data))
	if !strings.Contains(bottom, "Grains: checked") || !strings.Contains(bottom, "q: quit") {
		t.Fatalf("scrolling should reveal last details line without losing footer:\n%s", bottom)
	}
	foundKernel := false
	for offset := 0; offset <= ScrollLimit(data, 2); offset++ {
		data.DetailOffset = offset
		if strings.Contains(ansi.Strip(Render(data)), "kernel: Linux") {
			foundKernel = true
			break
		}
	}
	if !foundKernel {
		t.Fatal("lower grains should be reachable with the Details search pinned")
	}
	data.Focus = 1
	data.OverviewOffset = 99
	if view := ansi.Strip(Render(data)); !strings.Contains(view, "Connection presence is not job") || !strings.Contains(view, "success.") {
		t.Fatalf("lower overview content is unreachable:\n%s", view)
	}
	data.Help = true
	data.HelpOffset = 99
	if view := ansi.Strip(Render(data)); !strings.Contains(view, "Grains inspection sends a read-only Salt job.") || !strings.Contains(view, "scroll help") {
		t.Fatalf("help should scroll inside the frame:\n%s", view)
	}
}

func TestAdditionalGrainsAreSortedNestedAndScrollIntoView(t *testing.T) {
	data := ViewData{Width: 120, Height: 8, Focus: 2, Selected: "web-01", Detail: Source{At: time.Now()},
		Grains: map[string]any{"os": "Ubuntu", "z-custom": map[string]any{"rack": json.Number("13"), "roles": []any{"web", "api"}}, "a-custom": true},
	}
	content := strings.Join(detailLines(data, 30), "\n")
	if !(strings.Index(content, "os: Ubuntu") < strings.Index(content, "a-custom: true") && strings.Index(content, "a-custom: true") < strings.Index(content, "z-custom: {")) || !strings.Contains(content, `"rack": 13`) {
		t.Fatalf("unexpected grain order or structured value:\n%s", content)
	}
	found := false
	for offset := 0; offset <= ScrollLimit(data, 2); offset++ {
		data.DetailOffset = offset
		view := ansi.Strip(Render(data))
		if strings.Contains(view, "roles") && strings.Contains(view, "q: quit") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("nested grain not reachable with footer pinned")
	}
	data.Grains["large"] = strings.Repeat("x", maxGrainValueBytes+100)
	if content := strings.Join(grainLines(data.Grains, ""), "\n"); !strings.Contains(content, "(truncated)") || strings.Contains(content, strings.Repeat("x", maxGrainValueBytes+1)) {
		t.Fatal("long grain value was not bounded")
	}
}

func TestDetailsSearchMatchesKeysNestedValuesAndShowsSummary(t *testing.T) {
	data := ViewData{Width: 120, Height: 12, Focus: 2, Selected: "node-01", Detail: Source{At: time.Now()},
		DetailSearch: "/ rack", DetailQuery: "rack", DetailSearching: true,
		Grains: map[string]any{"fqdn": "node.example.org", "os": "Ubuntu", "osrelease": "24.04", "os_family": "Debian",
			"ipv4": []any{"10.0.0.4", "127.0.0.1"}, "ipv6": []any{"::1"},
			"site": map[string]any{"rack": "A7"}, "other": "unused"},
	}
	content := strings.Join(detailLines(data, 36), "\n")
	for _, want := range []string{"Hostname: node.example.org", "OS: Ubuntu", "OS family: Debian", "IPv4: 10.0.0.4", "IPv6: ::1", "site: {", `"rack": "A7"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("details missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "other: unused") || strings.Contains(content, "fqdn: node.example.org") {
		t.Fatalf("grain filter did not exclude unrelated entries:\n%s", content)
	}
	data.DetailQuery = "no-such-value"
	if content = strings.Join(detailLines(data, 36), "\n"); !strings.Contains(content, "No grains match") || !strings.Contains(content, "Hostname: node.example.org") {
		t.Fatalf("no-match view lost context:\n%s", content)
	}
	if view := ansi.Strip(Render(data)); !strings.Contains(view, "no matches") {
		t.Fatalf("no-match state must remain visible above long detail content:\n%s", view)
	}
	data.DetailQuery = "ubuntu"
	if content = strings.Join(detailLines(data, 36), "\n"); !strings.Contains(content, "os: Ubuntu") || strings.Contains(content, "site: {") {
		t.Fatalf("grain value matching failed:\n%s", content)
	}
	data.DetailQuery = ""
	data.Grains = map[string]any{"site": "A7"}
	if content = strings.Join(detailLines(data, 36), "\n"); !strings.Contains(content, "IPv4: unavailable") || !strings.Contains(content, "IPv6: unavailable") {
		t.Fatalf("missing IP values must be explicit:\n%s", content)
	}
}

func TestFilteredDetailsPageCapacityAndTerminalFit(t *testing.T) {
	data := ViewData{Width: 36, Height: 9, Focus: 2, Selected: "node", Detail: Source{At: time.Now()},
		DetailSearch: "/ rack", DetailQuery: "rack", Grains: map[string]any{"site": map[string]any{"rack": "A7"}},
	}
	if PageRows(data.Height, 2, true) != 2 || ScrollLimit(data, 2) == 0 {
		t.Fatal("details should reserve one pinned search row and scroll remaining content")
	}
	for _, height := range []int{9, 8, 7, 5, 3} {
		data.Height = height
		data.DetailOffset = ScrollLimit(data, 2)
		view := Render(data)
		if len(strings.Split(view, "\n")) != height {
			t.Fatalf("details overflow at height %d", height)
		}
		for _, row := range strings.Split(view, "\n") {
			if lipgloss.Width(row) != data.Width {
				t.Fatalf("details width overflow at height %d: %q", height, ansi.Strip(row))
			}
		}
	}
}

func TestSelectedRowSurvivesShortResize(t *testing.T) {
	data := ViewData{Width: 36, Focus: 0, Search: "/ ", Offset: 0,
		Rows: []Row{{ID: "alpha"}, {ID: "bravo"}, {ID: "charlie", Selected: true}},
	}
	for _, height := range []int{8, 7, 6, 5, 4, 3} {
		data.Height = height
		if view := ansi.Strip(Render(data)); !strings.Contains(view, "charlie") {
			t.Fatalf("selected minion hidden at %dx%d:\n%s", data.Width, height, view)
		}
	}
}

func TestUpdateNoticeIsCompactAndHiddenWhileSearching(t *testing.T) {
	data := ViewData{Width: 200, Height: 20, RunningVersion: "v1.2.3", AvailableUpdate: "v1.2.4"}
	view := ansi.Strip(Render(data))
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[1], "v1.2.4 available") || !strings.Contains(lines[1], "U: update") {
		t.Fatalf("update notice missing from its own header row:\n%s", view)
	}
	if strings.Contains(lines[len(lines)-1], "warning") {
		t.Fatalf("notice must not read as a warning banner")
	}
	if !strings.Contains(lines[len(lines)-1], "U: update") {
		t.Fatalf("footer should advertise the update shortcut:\n%s", view)
	}
	data.Searching = true
	view = ansi.Strip(Render(data))
	if strings.Contains(view, "available") || strings.Contains(view, "U: update") {
		t.Fatalf("a focused search must not advertise the update action:\n%s", view)
	}
}

func TestUpdateNoticeNeverBreaksFixedWidthOrHeight(t *testing.T) {
	data := ViewData{RunningVersion: "v1.2.3", AvailableUpdate: "v1.2.4"}
	for _, width := range []int{120, 80, 36, 20, 5, 1} {
		for _, height := range []int{20, 8, 4, 3, 2, 1} {
			data.Width, data.Height = width, height
			view := Render(data)
			lines := strings.Split(view, "\n")
			if len(lines) != height {
				t.Fatalf("%dx%d: got %d rows", width, height, len(lines))
			}
			for _, line := range lines {
				if cells := lipgloss.Width(line); cells != width {
					t.Fatalf("%dx%d: row is %d cells: %q", width, height, cells, ansi.Strip(line))
				}
			}
		}
	}
}
