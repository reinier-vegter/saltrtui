package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

type ConsoleEntry struct {
	Command, Stdout, Stderr string
	Retcode                 int
	Err                     error
	At                      time.Time
}

type ConsoleViewData struct {
	Width, Height, Offset int
	Context, ID           string
	Account, Cwd          string
	Input, Search, Match  string
	Searching             bool
	Busy, IdentityBusy    bool
	HistoryBusy           bool
	IdentityErr           error
	HistoryErr            error
	Saved, Session        int
	Entries               []ConsoleEntry
}

const maxConsoleField = 16 << 10

func consoleOutput(raw string, width int) []string {
	if len(raw) > maxConsoleField {
		raw = strings.ToValidUTF8(raw[:maxConsoleField], "") + "\n… (display truncated)"
	}
	return styledLines(strings.Split(strings.TrimSuffix(raw, "\n"), "\n"), width)
}

func consoleLines(v ConsoleViewData, width int) []string {
	var lines []string
	for _, entry := range v.Entries {
		lines = append(lines, accent.Render(clip("$ "+entry.Command, width)))
		if entry.Err != nil {
			lines = append(lines, failure.Render(clip("Outcome unknown/error: "+entry.Err.Error(), width)))
		} else {
			if entry.Stdout != "" {
				lines = append(lines, consoleOutput(entry.Stdout, width)...)
			}
			if entry.Stderr != "" {
				lines = append(lines, muted.Render("stderr:"))
				lines = append(lines, consoleOutput(entry.Stderr, width)...)
			}
			status := fmt.Sprintf("exit %d", entry.Retcode)
			if entry.Retcode != 0 {
				lines = append(lines, failure.Render(status))
			} else {
				lines = append(lines, muted.Render(status))
			}
		}
		lines = append(lines, "")
	}
	if len(lines) == 0 {
		lines = styledLines([]string{"Enter a command to run one finite Salt job on this minion.", "Commands that require a live terminal or stdin are not supported."}, width)
	}
	return lines
}

func ConsoleScrollLimit(v ConsoleViewData) int {
	return max(0, len(consoleLines(v, max(1, v.Width-4)))-max(1, v.Height-8))
}

// RenderConsole fills the terminal with a scrollable transcript and pinned input.
func RenderConsole(v ConsoleViewData) string {
	w, h := max(0, v.Width), max(0, v.Height)
	if w == 0 || h == 0 {
		return ""
	}
	hints := []hint{{"enter", "run"}, {"↑↓", "history"}, {"ctrl+r", "search"}, {"pgup/down", "transcript"}, {"esc", "Fleet"}, {"ctrl+c", "quit"}}
	notice := ""
	if v.Searching {
		hints, notice = []hint{{"ctrl+r", "older"}, {"enter", "run match"}, {"esc", "restore draft"}}, "Reverse search"
	} else if v.Busy {
		hints, notice = []hint{{"esc", "Fleet"}}, "Waiting for Salt return; outcome unknown until reply"
	}
	if h == 1 {
		return actionBar(w, notice, false, hints...)
	}
	account, cwd := v.Account, v.Cwd
	if account == "" {
		account = "account unknown"
	}
	if cwd == "" {
		cwd = "cwd unknown"
	}
	frame := modeFrame(w, h, "Fleet", v.Context, "Console · "+v.ID)
	bodyHeight := h - len(frame) - 1
	if bodyHeight > 0 {
		input := v.Input
		if v.Searching {
			input = v.Search + "  → " + clean(ansi.Strip(v.Match))
		}
		panelHeight := bodyHeight
		if bodyHeight >= 4 {
			panelHeight--
		}
		pinned := []string{}
		if panelHeight >= 5 {
			status := fmt.Sprintf("%s · %s · saved Bash history: %d · session: %d", account, cwd, v.Saved, v.Session)
			if v.IdentityBusy || v.HistoryBusy {
				status += " · fetching"
			}
			if v.IdentityErr != nil {
				status += " · identity unavailable: " + clean(v.IdentityErr.Error())
			} else if v.HistoryErr != nil {
				status += " · saved history unavailable: " + clean(v.HistoryErr.Error())
			}
			pinned = append(pinned, muted.Render(clip(status, w-4)))
		}
		body := panel("Transcript · "+v.Context, consoleLines(v, w-4), v.Offset, w, panelHeight, true, pinned)
		frame = append(frame, strings.Split(body, "\n")...)
		if panelHeight < bodyHeight {
			// Bubbles' text input includes ANSI cursor styling. Truncating it as
			// plain text drops ESC while leaving visible fragments like "[37m".
			frame = append(frame, accent.Render(fixed(input, w)))
		}
	}
	frame = append(frame, actionBar(w, notice, false, hints...))
	return strings.Join(frame, "\n")
}
