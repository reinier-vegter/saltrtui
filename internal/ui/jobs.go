package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type JobSummary struct{ JID, Function, Target, TargetType, StartTime string }
type JobReturn struct {
	Value   json.RawMessage
	Retcode *int
	Success *bool
}
type JobDetail struct {
	JobSummary
	Minions []string
	Returns map[string]JobReturn
}

type JobsViewData struct {
	Width, Height, Focus, Offset, DetailOffset, HelpOffset int
	Help, Searching                                        bool
	Context, Query, Search, Selected                       string
	Items                                                  []JobSummary
	List, Detail                                           Source
	Job                                                    JobDetail
}

func jobListLines(v JobsViewData, width, capacity int) ([]string, int, string) {
	if len(v.Items) == 0 {
		text := "No cached jobs found"
		switch {
		case v.List.Busy && v.List.At.IsZero():
			text = "Loading recent jobs…"
		case v.List.Err != nil && v.List.At.IsZero():
			text = "Jobs unavailable; press r to retry"
		case v.List.At.IsZero():
			text = "Jobs not loaded"
		case v.Query != "":
			text = "No matching jobs; clear the search"
		}
		return []string{muted.Render(clip(text, width))}, 0, "Jobs 0/0"
	}
	index := 0
	for i, job := range v.Items {
		if job.JID == v.Selected {
			index = i
			break
		}
	}
	offset := min(max(0, v.Offset), max(0, len(v.Items)-capacity))
	if index < offset {
		offset = index
	} else if index >= offset+capacity {
		offset = index - capacity + 1
	}
	lines := make([]string, 0, len(v.Items))
	for _, job := range v.Items {
		line := clean(job.JID + "  " + job.Function + "  " + job.Target + "  " + job.StartTime)
		if job.JID == v.Selected {
			lines = append(lines, selected.Width(width).Render(fixed(clip("> "+line, width), width)))
		} else {
			lines = append(lines, item.Render(clip("  "+line, width)))
		}
	}
	return lines, offset, fmt.Sprintf("Jobs %d/%d", index+1, len(v.Items))
}

func jobDetailLines(v JobsViewData, width int) []string {
	if v.Selected == "" {
		return styledLines([]string{"Select a cached job to inspect its returns."}, width)
	}
	lines := []string{"JID: " + v.Selected}
	if v.Job.JID == v.Selected && !v.Detail.At.IsZero() {
		job := v.Job
		lines = append(lines, "Function: "+job.Function, "Target: "+job.Target,
			"Target type: "+job.TargetType, "Started: "+job.StartTime,
			fmt.Sprintf("Expected minions: %d", len(job.Minions)),
			fmt.Sprintf("Cached returns: %d", len(job.Returns)), "")
		ids := make([]string, 0, len(job.Minions)+len(job.Returns))
		seen := make(map[string]bool)
		for _, id := range job.Minions {
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
		for id := range job.Returns {
			if !seen[id] {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		for _, id := range ids {
			ret, ok := job.Returns[id]
			if !ok {
				lines = append(lines, id+": no return recorded")
				continue
			}
			status := id + ": cached return"
			if ret.Retcode != nil {
				status += fmt.Sprintf(" · retcode %d", *ret.Retcode)
			}
			if ret.Success != nil {
				status += fmt.Sprintf(" · success %t", *ret.Success)
			}
			lines = append(lines, status)
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, ret.Value, "", "  "); err != nil {
				pretty.Write(ret.Value)
			}
			value := pretty.String()
			if len(value) > maxGrainValueBytes {
				value = strings.ToValidUTF8(value[:maxGrainValueBytes], "") + "… (truncated)"
			}
			for part := range strings.SplitSeq(value, "\n") {
				lines = append(lines, "  "+part)
			}
		}
		if len(ids) == 0 {
			lines = append(lines, "No cached minion returns or expected minions recorded.")
		}
	} else if v.Detail.At.IsZero() && !v.Detail.Busy && v.Detail.Err == nil {
		lines = append(lines, "Press enter to load cached returns")
	}
	lines = append(lines, sourceLine("Job detail", v.Detail))
	return styledLines(lines, width)
}

func JobsScrollLimit(v JobsViewData, which int) int {
	if v.Width < 6 || v.Height < 6 {
		return 0
	}
	width := v.Width
	if which == 1 && v.Width >= 68 {
		width = v.Width - (v.Width-2)/2 - 2
	}
	var lines []string
	if which == 2 {
		lines = styledLines([]string{"Jobs navigation", "", "1 Fleet  2 Jobs  4 Events  tab switch panels", "/ search JID/function/target  enter inspect job", "r refresh focused panel  pgup/pgdown page", "Missing returns do not imply failure or completion."}, width-4)
	} else if which == 1 {
		lines = jobDetailLines(v, width-4)
	} else {
		return 0
	}
	return max(0, len(lines)-PageRows(v.Height, 1, false))
}

func RenderJobs(v JobsViewData) string {
	w, h := max(0, v.Width), max(0, v.Height)
	if w == 0 || h == 0 {
		return ""
	}
	hints := []hint{{"↑↓", "move/scroll"}, {"/", "search"}, {"enter", "inspect"}, {"tab", "panel"}, {"r", "refresh"}, {"?", "help"}, {"q", "quit"}}
	notice := ""
	alert := false
	if v.Searching {
		hints = []hint{{"enter/esc", "close search"}, {"ctrl+c", "quit"}}
		notice = "Filter jobs"
	} else if v.Help {
		hints = []hint{{"↑↓", "scroll help"}, {"esc/?", "return"}, {"ctrl+c", "quit"}}
	} else if v.List.Err != nil {
		hints, notice, alert = []hint{{"r", "retry"}, {"?", "help"}}, "Jobs: "+v.List.Err.Error(), true
	} else if v.Focus == 1 && v.Detail.Err != nil {
		hints, notice, alert = []hint{{"r", "retry"}, {"?", "help"}}, "Job detail stale/error", true
	}
	if h == 1 {
		return actionBar(w, notice, alert, hints...)
	}
	frame := []string{modeBar(w, "Jobs", v.Context, "")}
	if h >= 4 {
		frame = append(frame, muted.Render(strings.Repeat("─", w)))
	}
	bodyHeight := h - len(frame) - 1
	if bodyHeight > 0 {
		var body string
		if v.Help {
			body = panel("Jobs keys", styledLines([]string{"Jobs navigation", "", "1 Fleet  2 Jobs  4 Events  tab switch panels", "/ search JID/function/target  enter inspect job", "r refresh focused panel  pgup/pgdown page", "Missing returns do not imply failure or completion."}, w-4), v.HelpOffset, w, bodyHeight, true, nil)
		} else {
			left := w
			if w >= 68 {
				left = (w - 2) / 2
			}
			list, offset, title := jobListLines(v, max(0, left-4), max(1, bodyHeight-4))
			var pinned []string
			if bodyHeight >= 5 {
				search := muted
				if v.Searching {
					search = accent
				}
				pinned = []string{search.Render(clip(ansi.Strip(v.Search), left-4))}
			} else if v.Searching {
				title = ansi.Strip(v.Search)
			}
			body = panel(title, list, offset, left, bodyHeight, v.Focus == 0, pinned)
			if w >= 68 {
				right := w - left - 2
				body = lipgloss.JoinHorizontal(lipgloss.Top, body, "  ", panel("Job detail", jobDetailLines(v, right-4), v.DetailOffset, right, bodyHeight, v.Focus == 1, nil))
			} else if v.Focus == 1 {
				body = panel("Job detail", jobDetailLines(v, w-4), v.DetailOffset, w, bodyHeight, true, nil)
			}
		}
		frame = append(frame, strings.Split(body, "\n")...)
	}
	frame = append(frame, actionBar(w, notice, alert, hints...))
	return strings.Join(frame, "\n")
}
