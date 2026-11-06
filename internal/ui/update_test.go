package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestUpdatePromptDimensionsAndLongDestination(t *testing.T) {
	for _, path := range []string{"/opt/saltrtui", "/usr/local/" + strings.Repeat("very-long-dir/", 30) + "saltrtui", "/東京/saltrtui"} {
		for _, size := range [][2]int{{1, 1}, {20, 3}, {40, 8}, {59, 11}, {60, 12}, {80, 18}, {120, 40}} {
			v := UpdateViewData{Width: size[0], Height: size[1], Phase: "confirm", Current: path, Running: "v1.0.0", Available: "v1.0.1"}
			view := RenderSelfUpdate(v)
			rows := strings.Split(view, "\n")
			if len(rows) != v.Height {
				t.Fatalf("height mismatch %v: %d", size, len(rows))
			}
			for _, row := range rows {
				if lipgloss.Width(row) != v.Width {
					t.Fatalf("width mismatch %v: %q", size, row)
				}
			}
			if !UpdatePromptLayout(v).FitsConfirmation && strings.Contains(ansi.Strip(view), "enter: confirm choice") {
				t.Fatal("blocked update advertised confirmation")
			}
		}
	}
}
