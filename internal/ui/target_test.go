package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestTargetFrameFitsTerminalAndShowsCandidateSource(t *testing.T) {
	v := TargetViewData{Context: "master-a", Mode: "glob", Input: "> web-*", Expression: "web-*", Preview: Source{At: time.Now()}, IDs: []string{"web-01", "web-02"}, Presence: Source{At: time.Now()}, Connected: map[string]bool{"web-01": true}}
	for _, size := range [][2]int{{120, 30}, {68, 12}, {30, 8}, {8, 3}, {2, 1}} {
		v.Width, v.Height = size[0], size[1]
		view := RenderTarget(v)
		lines := strings.Split(view, "\n")
		if len(lines) != v.Height {
			t.Fatalf("%dx%d: got %d lines", v.Width, v.Height, len(lines))
		}
		for _, line := range lines {
			if width := ansi.StringWidth(line); width != v.Width {
				t.Fatalf("%dx%d: rendered width %d: %q", v.Width, v.Height, width, line)
			}
		}
	}
	v.Width, v.Height = 120, 30
	if text := ansi.Strip(RenderTarget(v)); !strings.Contains(text, "matches among accepted minion keys") || !strings.Contains(text, "not observed") {
		t.Fatalf("scope source or presence missing: %s", text)
	}
}

func TestTargetScrollRevealsFocusedMatch(t *testing.T) {
	v := TargetViewData{Width: 36, Height: 10, Mode: "list", Input: "> web-01", ResultsFocus: true, IDs: []string{"web-01", "web-02", "web-03"}, Preview: Source{At: time.Now()}}
	v.Cursor = 2
	v.Offset = TargetCursorOffset(v)
	if v.Offset == 0 || !strings.Contains(ansi.Strip(RenderTarget(v)), "> web-03") {
		t.Fatal("focused result not visible in short terminal")
	}
}
