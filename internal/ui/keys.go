package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type KeyRow struct {
	ID, State string
	Selected  bool
}

type KeysViewData struct {
	Width, Height, Focus, Offset, DetailOffset, HelpOffset int
	Context, Query, Search, Fingerprint, Confirm, Status   string
	Searching, Help, Busy, Ambiguous                       bool
	Selected                                               KeyRow
	Rows                                                   []KeyRow
	List, Detail                                           Source
	FileTime                                               time.Time
}

func keyListLines(v KeysViewData, width, capacity int) ([]string, int, string) {
	if len(v.Rows) == 0 {
		text := "No minion keys found"
		switch {
		case v.List.Busy && v.List.At.IsZero():
			text = "Loading master keys…"
		case v.List.Err != nil && v.List.At.IsZero():
			text = "Keys unavailable; press r to retry"
		case v.List.At.IsZero():
			text = "Keys not loaded"
		case v.Query != "":
			text = "No matching keys; clear the search"
		}
		return []string{muted.Render(clip(text, width))}, 0, "Keys 0/0"
	}
	index := 0
	for i, row := range v.Rows {
		if row.Selected {
			index = i
			break
		}
	}
	offset := max(0, min(v.Offset, max(0, len(v.Rows)-capacity)))
	if index < offset {
		offset = index
	} else if index >= offset+capacity {
		offset = index - capacity + 1
	}
	lines := make([]string, 0, len(v.Rows))
	for _, row := range v.Rows {
		text := clean(row.ID + " · " + row.State)
		if row.Selected {
			lines = append(lines, selected.Width(width).Render(fixed(clip("> "+text, width), width)))
		} else {
			lines = append(lines, item.Render(clip("  "+text, width)))
		}
	}
	return lines, offset, fmt.Sprintf("Keys %d/%d", index+1, len(v.Rows))
}

func keyDetailLines(v KeysViewData, width int) []string {
	if v.Selected.ID == "" {
		return styledLines([]string{"Select a key to inspect its details."}, width)
	}
	lines := []string{"ID: " + v.Selected.ID, "State: " + v.Selected.State,
		"Inventory: " + sourceLine("Master keys", v.List), ""}
	if v.Fingerprint == "" {
		lines = append(lines, "Fingerprint: Unknown (press enter to inspect)")
	} else {
		lines = append(lines, "Fingerprint: "+v.Fingerprint)
	}
	if v.Selected.State == "pending" {
		timeText := "Unknown"
		if !v.FileTime.IsZero() {
			timeText = v.FileTime.Local().Format(time.RFC3339)
		}
		lines = append(lines, "Announced: Unknown (Salt key listing has no announcement time)",
			"Approx. key-file modified: "+timeText,
			"File time is not proof of first announcement.")
	}
	lines = append(lines, sourceLine("Fingerprint read", v.Detail), "")
	if v.Ambiguous {
		lines = append(lines, "This ID occurs in multiple key states; actions disabled.")
	}
	switch v.Selected.State {
	case "pending":
		lines = append(lines, "a accept · b block (reject)")
	case "accepted":
		lines = append(lines, "d deny/revoke (delete accepted key)", "A revoked minion may announce again.")
	case "rejected":
		lines = append(lines, "Blocked: explicitly rejected on master.")
	case "denied":
		lines = append(lines, "Denied automatically by Salt (e.g. duplicate ID).")
	}
	if v.Confirm != "" {
		lines = append(lines, "", "Confirm "+v.Confirm+" key "+v.Selected.ID+" ("+v.Selected.State+")?",
			"y confirm · n/esc cancel", "Rechecks state before sending the action.")
	}
	return styledLines(lines, width)
}

func keyHelp() []string {
	return []string{"Master key management", "", "6 Keys  1 Fleet  2 Jobs  4 Events",
		"/ search IDs/states  tab switch panels", "enter inspect key fingerprint", "a accept pending  b block pending (reject)",
		"d deny/revoke accepted (delete)", "y confirm  n/esc cancel", "r refresh keys  pgup/pgdown page", "? help  q quit", "",
		"Denied is an automatic Salt key state, not revoke.",
		"Pending minions cannot report remote grains.", "Key-file mtime is not an announcement timestamp."}
}

func KeysScrollLimit(v KeysViewData, which int) int {
	if v.Width < 6 || v.Height < 6 {
		return 0
	}
	w := v.Width
	if which == 1 && w >= 68 {
		w = w - (w-2)/2 - 2
	}
	lines := keyDetailLines(v, w-4)
	if which == 2 {
		lines = styledLines(keyHelp(), v.Width-4)
	}
	return max(0, len(lines)-PageRows(v.Height, 1, false))
}

func RenderKeys(v KeysViewData) string {
	w, h := max(0, v.Width), max(0, v.Height)
	if w == 0 || h == 0 {
		return ""
	}
	hints := []hint{{"a", "accept"}, {"b", "block"}, {"d", "deny/revoke"}, {"/", "search"}, {"enter", "details"}, {"r", "refresh"}, {"?", "help"}, {"q", "quit"}}
	notice := ""
	alert := false
	switch {
	case v.Help:
		hints = []hint{{"↑↓", "scroll help"}, {"esc/?", "return"}, {"ctrl+c", "quit"}}
	case v.Searching:
		hints, notice = []hint{{"enter/esc", "close search"}, {"ctrl+c", "quit"}}, "Filter keys"
	case v.Confirm != "":
		hints, notice, alert = []hint{{"y", "confirm"}, {"n/esc", "cancel"}}, "Confirm "+v.Confirm+" "+v.Selected.ID+" ("+v.Selected.State+")?", true
	case v.List.Err != nil:
		hints, notice, alert = []hint{{"r", "retry"}, {"?", "help"}}, "Keys: "+v.List.Err.Error(), true
	case v.Busy:
		hints, notice = []hint{{"?", "help"}}, "Key action in progress; wait for refresh"
	case v.Status != "":
		hints, notice = []hint{{"r", "refresh"}, {"?", "help"}}, v.Status
		alert = !strings.HasSuffix(v.Status, "verified in refreshed keys")
	}
	if h == 1 {
		return actionBar(w, notice, alert, hints...)
	}
	frame := []string{modeBar(w, "Keys", v.Context, "")}
	if h >= 4 {
		frame = append(frame, muted.Render(strings.Repeat("─", w)))
	}
	bodyHeight := h - len(frame) - 1
	if bodyHeight > 0 {
		var body string
		if v.Help {
			body = panel("Keys help", styledLines(keyHelp(), w-4), v.HelpOffset, w, bodyHeight, true, nil)
		} else {
			left := w
			if w >= 68 {
				left = (w - 2) / 2
			}
			list, offset, title := keyListLines(v, max(0, left-4), max(1, bodyHeight-4))
			var pinned []string
			if bodyHeight >= 5 {
				searchStyle := muted
				if v.Searching {
					searchStyle = accent
				}
				pinned = []string{searchStyle.Render(clip(ansi.Strip(v.Search), left-4))}
			} else if v.Searching {
				title = ansi.Strip(v.Search)
			}
			body = panel(title, list, offset, left, bodyHeight, v.Focus == 0, pinned)
			if w >= 68 {
				right := w - left - 2
				body = lipgloss.JoinHorizontal(lipgloss.Top, body, "  ", panel("Key detail", keyDetailLines(v, right-4), v.DetailOffset, right, bodyHeight, v.Focus == 1, nil))
			} else if v.Focus == 1 {
				body = panel("Key detail", keyDetailLines(v, w-4), v.DetailOffset, w, bodyHeight, true, nil)
			}
		}
		frame = append(frame, strings.Split(body, "\n")...)
	}
	frame = append(frame, actionBar(w, notice, alert, hints...))
	return strings.Join(frame, "\n")
}
