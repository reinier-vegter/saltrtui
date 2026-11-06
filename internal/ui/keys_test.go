package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestKeyPanelLayoutAndPendingDetail(t *testing.T) {
	v := KeysViewData{Context: "/etc/salt", BackendAvailable: true, Selected: KeyRow{ID: "new-1", State: "pending"},
		Rows: []KeyRow{{ID: "new-1", State: "pending", Selected: true}},
		List: Source{At: time.Now()}, Detail: Source{At: time.Now()}, Fingerprint: "ab:cd"}
	for _, w := range []int{120, 80, 36, 5, 1} {
		for _, h := range []int{25, 8, 4, 1} {
			for _, focus := range []int{0, 1} {
				for _, help := range []bool{false, true} {
					v.Width, v.Height, v.Focus, v.Help = w, h, focus, help
					lines := strings.Split(RenderKeys(v), "\n")
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
	v.Width, v.Height, v.Focus, v.Help = 110, 28, 1, false
	text := ansi.Strip(RenderKeys(v))
	for _, want := range []string{"Fingerprint: ab:cd", "Announced: Unknown", "Approx. key-file modified: Unknown", "Pending: awaiting a decision on the master.", "A: accept key"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	v.Confirm = "accept"
	v.ConfirmedKey = v.Selected
	if text := ansi.Strip(RenderKeys(v)); !strings.Contains(text, "Confirm accept key") || !strings.Contains(text, "Key: new-1") {
		t.Fatal("confirmation not visible in bottom box")
	}
}

func TestKeysKeepsSelectedRowVisibleWhenOnlyPanelHeadingFits(t *testing.T) {
	rows := make([]KeyRow, 40)
	for i := range rows {
		rows[i] = KeyRow{ID: fmt.Sprintf("key-%02d", i), State: "pending", Selected: i == 30}
	}
	v := KeysViewData{Width: 100, Height: 6, BackendAvailable: true, Selected: rows[30], Rows: rows, List: Source{At: time.Now()}}
	for _, searching := range []bool{false, true} {
		v.Searching = searching
		if searching {
			v.Search = "/ key-30"
		}
		view := ansi.Strip(RenderKeys(v))
		if len(strings.Split(view, "\n")) != v.Height || !strings.Contains(view, "> key-30 · pending") {
			t.Fatalf("selected key disappeared at short height (searching=%v):\n%s", searching, view)
		}
	}
}
