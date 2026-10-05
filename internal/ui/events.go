package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type EventRow struct {
	Tag, Minion, JID, Stamp string
	Data                    json.RawMessage
	Observed                time.Time
}

type EventsViewData struct {
	Width, Height, Focus, Offset, DetailOffset, HelpOffset int
	Context, Family, Query, Search, Status                 string
	Searching, Help, Paused                                bool
	Unseen, Dropped                                        uint64
	Selected, Inspected                                    int
	Rows                                                   []EventRow
}

func eventListLines(v EventsViewData, width, capacity int) ([]string, int, string) {
	if len(v.Rows) == 0 {
		text := "No events captured since app launch…"
		if v.Status != "" {
			text = "Listener unavailable; press r to retry"
		}
		if v.Query != "" || v.Family != "all" {
			text = "No matching buffered events; change filter"
		}
		return []string{muted.Render(clip(text, width))}, 0, "Events 0/0"
	}
	idx := max(0, min(v.Selected, len(v.Rows)-1))
	offset := max(0, min(v.Offset, max(0, len(v.Rows)-capacity)))
	if idx < offset {
		offset = idx
	} else if idx >= offset+capacity {
		offset = idx - capacity + 1
	}
	lines := make([]string, 0, len(v.Rows))
	for i, row := range v.Rows {
		line := row.Observed.Local().Format("15:04:05") + "  " + row.Tag
		if row.Minion != "" {
			line += "  " + row.Minion
		}
		if row.JID != "" && !strings.Contains(row.Tag, row.JID) {
			line += "  " + row.JID
		}
		if i == idx {
			lines = append(lines, selected.Width(width).Render(fixed(clip("> "+line, width), width)))
		} else {
			lines = append(lines, item.Render(clip("  "+line, width)))
		}
	}
	return lines, offset, fmt.Sprintf("Events %d/%d", idx+1, len(v.Rows))
}

func eventDetailLines(v EventsViewData, width int) []string {
	if v.Inspected < 0 || v.Inspected >= len(v.Rows) {
		return styledLines([]string{"Select an event and press enter to inspect its payload.",
			"Event payloads can contain command arguments and return data."}, width)
	}
	row := v.Rows[v.Inspected]
	lines := []string{"Tag: " + row.Tag, "Observed: " + row.Observed.Local().Format(time.RFC3339)}
	if row.Stamp != "" {
		lines = append(lines, "Event _stamp: "+row.Stamp)
	}
	if row.Minion != "" {
		lines = append(lines, "Minion: "+row.Minion)
	}
	if row.JID != "" {
		lines = append(lines, "JID: "+row.JID)
	}
	lines = append(lines, "", "Payload:")
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, row.Data, "", "  "); err != nil {
		pretty.Write(row.Data)
	}
	for part := range strings.SplitSeq(pretty.String(), "\n") {
		lines = append(lines, part)
	}
	return styledLines(lines, width)
}

func eventsHelp() []string {
	return []string{"Live master events", "", "4 Events  1 Fleet  2 Jobs  esc leave Events",
		"tab switch list/detail  enter inspect payload", "/ filter tag, minion, JID, payload  pgup/pgdown page",
		"a all  g jobs  m minions  K keys  p presence", "space pause/resume auto-follow  c clear local buffer",
		"r restart subscription (keep buffer)  ? help  ctrl+c quit", "", "Capture starts when the app launches and continues in other views.",
		"No replay before launch or during gaps; drops are reported.",
		"Presence/state progress events depend on Salt configuration."}
}

func EventsScrollLimit(v EventsViewData, which int) int {
	if v.Width < 6 || v.Height < 6 {
		return 0
	}
	w := v.Width
	if which == 1 && w >= 68 {
		w = w - (w-2)/2 - 2
	}
	lines := eventDetailLines(v, w-4)
	if which == 2 {
		lines = styledLines(eventsHelp(), v.Width-4)
	}
	return max(0, len(lines)-PageRows(v.Height, 1, false))
}

func RenderEvents(v EventsViewData) string {
	w, h := max(0, v.Width), max(0, v.Height)
	if w == 0 || h == 0 {
		return ""
	}
	hints := []hint{{"a/g/m/K/p", "categories"}, {"space", "pause"}, {"/", "filter"}, {"enter", "inspect"}, {"r", "restart"}, {"?", "help"}, {"q", "quit"}}
	notice := ""
	alert := false
	if v.Searching {
		hints, notice = []hint{{"enter/esc", "close search"}, {"ctrl+c", "quit"}}, "Filter events"
	} else if v.Help {
		hints = []hint{{"↑↓", "scroll help"}, {"esc/?", "return"}, {"ctrl+c", "quit"}}
	} else if v.Status != "" {
		hints, notice, alert = []hint{{"r", "restart"}, {"?", "help"}}, "Events: "+v.Status, true
	}
	if v.Dropped > 0 {
		notice = fmt.Sprintf("dropped %d · %s", v.Dropped, notice)
		alert = true
	}
	if v.Paused {
		notice = fmt.Sprintf("paused +%d · %s", v.Unseen, notice)
	}
	if h == 1 {
		return actionBar(w, notice, alert, hints...)
	}
	frame := []string{modeBar(w, "Events", v.Context, "")}
	if h >= 4 {
		frame = append(frame, muted.Render(strings.Repeat("─", w)))
	}
	bodyHeight := h - len(frame) - 1
	if bodyHeight > 0 {
		var body string
		if v.Help {
			body = panel("Events keys", styledLines(eventsHelp(), w-4), v.HelpOffset, w, bodyHeight, true, nil)
		} else {
			left := w
			if w >= 68 {
				left = (w - 2) / 2
			}
			list, offset, title := eventListLines(v, max(0, left-4), max(1, bodyHeight-4))
			title += " · " + v.Family
			if v.Paused {
				title += fmt.Sprintf(" · paused +%d", v.Unseen)
			}
			if v.Dropped > 0 {
				title += fmt.Sprintf(" · dropped %d", v.Dropped)
			}
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
				body = lipgloss.JoinHorizontal(lipgloss.Top, body, "  ", panel("Event detail", eventDetailLines(v, right-4), v.DetailOffset, right, bodyHeight, v.Focus == 1, nil))
			} else if v.Focus == 1 {
				body = panel("Event detail", eventDetailLines(v, w-4), v.DetailOffset, w, bodyHeight, true, nil)
			}
		}
		frame = append(frame, strings.Split(body, "\n")...)
	}
	frame = append(frame, actionBar(w, notice, alert, hints...))
	return strings.Join(frame, "\n")
}
