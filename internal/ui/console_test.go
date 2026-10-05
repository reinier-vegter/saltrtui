package ui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	"github.com/charmbracelet/x/ansi"
)

func TestConsoleTerminalFitAndTranscript(t *testing.T) {
	for _, size := range [][2]int{{120, 25}, {72, 12}, {26, 6}, {8, 2}} {
		v := ConsoleViewData{Width: size[0], Height: size[1], ID: "web-01", Account: "root", Cwd: "/srv", Input: "$ echo", Entries: []ConsoleEntry{{Command: "echo test", Stdout: "hello", Stderr: "notice", Retcode: 1}}}
		lines := strings.Split(RenderConsole(v), "\n")
		if len(lines) != v.Height {
			t.Fatalf("%dx%d rendered %d rows", v.Width, v.Height, len(lines))
		}
		for _, line := range lines {
			if ansi.StringWidth(line) != v.Width {
				t.Fatalf("%dx%d rendered width %d: %q", v.Width, v.Height, ansi.StringWidth(line), line)
			}
		}
		if size[1] >= 25 && (!strings.Contains(RenderConsole(v), "stderr:") || !strings.Contains(RenderConsole(v), "exit 1")) {
			t.Fatal("transcript lost stderr or exit code")
		}
	}
}

func TestConsoleRecalledHistoryKeepsInputStylingIntact(t *testing.T) {
	input := textinput.New()
	input.Prompt = "$ "
	input.SetWidth(42)
	input.Focus()
	input.SetValue("vim /etc/group")
	v := ConsoleViewData{Width: 72, Height: 12, ID: "web-01", Input: input.View()}
	for _, searching := range []bool{false, true} {
		v.Searching = searching
		if searching {
			v.Search, v.Match = input.View(), "vim /etc/group"
		}
		view := RenderConsole(v)
		plain := ansi.Strip(view)
		if !strings.Contains(plain, "vim /etc/group") || strings.Contains(plain, "[37m") || strings.Contains(plain, "[7;37m") {
			t.Fatalf("history prompt contains broken ANSI fragments: %q", plain)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) != v.Width {
				t.Fatalf("styled prompt changed terminal width: %q", line)
			}
		}
	}
}
