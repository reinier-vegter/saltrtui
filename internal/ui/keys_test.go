package ui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestKeyPanelLayoutAndPendingDetail(t *testing.T) {
	v := KeysViewData{Context: "/etc/salt", Selected: KeyRow{ID: "new-1", State: "pending"},
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
	for _, want := range []string{"Fingerprint: ab:cd", "Announced: Unknown", "Approx. key-file modified: Unknown", "a accept"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	v.Confirm = "accept"
	if text := ansi.Strip(RenderKeys(v)); !strings.Contains(text, "Confirm accept key new-1") {
		t.Fatal("confirmation not visible in detail")
	}
}
