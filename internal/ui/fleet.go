// Package ui renders application-facing Fleet data without Salt dependencies.
package ui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type Source struct {
	Count int
	At    time.Time
	Err   error
	Busy  bool
}
type Row struct {
	ID       string
	Status   string
	Selected bool
}
type ViewData struct {
	Context, Search, Query, Selected, SelectedStatus string
	DetailSearch, DetailQuery                        string
	Width, Height, Focus, Offset                     int
	OverviewOffset, DetailOffset, HelpOffset         int
	Help, Searching, DetailSearching                 bool
	Rows                                             []Row
	Inventory, Presence, Detail                      Source
	Connected, NotObserved                           int
	Grains                                           map[string]any
	RunningVersion, AvailableUpdate                  string
}

var (
	accent      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	muted       = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	item        = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	failure     = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	selected    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("39"))
	noticeStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
)

func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || !unicode.IsPrint(r) {
			return ' '
		}
		return r
	}, s)
}

func clip(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(clean(s), width, "…")
}

func fixed(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = ansi.Truncate(s, width, "…")
	return s + strings.Repeat(" ", max(0, width-ansi.StringWidth(s)))
}

func stamp(s Source) string {
	if s.At.IsZero() {
		return "not checked"
	}
	return "checked " + s.At.Local().Format("15:04:05")
}

func sourceLine(name string, s Source) string {
	state := stamp(s)
	if s.Busy {
		state += " · loading"
	} else if s.Err != nil {
		state += " · stale/error: " + clean(strings.TrimPrefix(s.Err.Error(), strings.ToLower(name)+": "))
	}
	return name + ": " + state
}

// ListRows is the number of selectable target rows at the current terminal
// height. The header, divider, footer, panel border, title, and search remain
// visible around the list.
func ListRows(height int) int {
	return max(1, height-7)
}

// PageRows mirrors the content capacity used by panel at the current height.
// Details reserves one additional row for its search field when visible.
func PageRows(height, which int, selected bool) int {
	if which == 0 || (which == 2 && selected && height >= 8) {
		return ListRows(height)
	}
	return max(1, height-6)
}

// TargetWidth is the outer width reserved for Targets at each breakpoint.
func TargetWidth(width int) int {
	switch {
	case width >= 100:
		return (width - 4) / 3
	case width >= 68:
		return (width - 2) / 2
	default:
		return max(0, width)
	}
}

// ScrollLimit gives the model the actual end of each rendered read-only pane;
// pressing Down at the end does not accumulate invisible scroll distance.
// Panel 3 is the help overlay.
func ScrollLimit(v ViewData, which int) int {
	w, h := max(0, v.Width), max(0, v.Height)
	if h < 6 || w < 6 {
		return 0
	}
	panelWidth := w
	if which != 3 {
		switch {
		case w >= 100:
			left := TargetWidth(w)
			middle := (w - 4 - left) / 2
			panelWidth = middle
			if which == 2 {
				panelWidth = w - 4 - left - middle
			}
		case w >= 68:
			panelWidth = w - 2 - TargetWidth(w)
		}
	}
	innerWidth := max(0, panelWidth-4)
	var count int
	switch which {
	case 1:
		count = len(overviewLines(v, innerWidth))
	case 2:
		count = len(detailLines(v, innerWidth))
	case 3:
		count = len(helpLines(innerWidth))
	default:
		return 0
	}
	return max(0, count-PageRows(h, which, v.Selected != ""))
}

func scroll(lines []string, offset, size int) ([]string, string) {
	if size <= 0 {
		return nil, ""
	}
	start := min(max(0, offset), max(0, len(lines)-size))
	end := min(len(lines), start+size)
	var hint string
	if len(lines) > size {
		hint = fmt.Sprintf(" %d-%d/%d", start+1, end, len(lines))
	}
	return lines[start:end], hint
}

// panel renders exactly width x height cells. Input lines are already styled;
// the panel itself owns its fixed-height border and empty-space fill.
func panel(title string, lines []string, offset, width, height int, focused bool, pinned []string) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if width < 6 || height < 3 {
		visible, _ := scroll(lines, offset, 1)
		plain := title
		if len(visible) > 0 {
			plain = ansi.Strip(visible[0])
		}
		result := make([]string, height)
		for i := range result {
			if i == 0 && height > 1 {
				result[i] = fixed(clip(title, width), width)
			} else {
				result[i] = fixed(clip(plain, width), width)
			}
		}
		return strings.Join(result, "\n")
	}
	innerWidth, innerHeight := width-4, height-2
	visible, hint := scroll(lines, offset, innerHeight-1-len(pinned))
	heading := clip(title+hint, innerWidth)
	if focused {
		heading = "▸ " + clip(title+hint, max(0, innerWidth-2))
	}
	content := []string{accent.Render(fixed(heading, innerWidth))}
	for _, line := range pinned {
		content = append(content, fixed(line, innerWidth))
	}
	for _, line := range visible {
		content = append(content, fixed(line, innerWidth))
	}
	for len(content) < innerHeight {
		content = append(content, strings.Repeat(" ", innerWidth))
	}
	border := lipgloss.Color("240")
	if focused {
		border = lipgloss.Color("39")
	}
	borderStyle := lipgloss.NewStyle().Foreground(border)
	result := make([]string, 0, height)
	result = append(result, borderStyle.Render("╭"+strings.Repeat("─", width-2)+"╮"))
	for _, line := range content {
		result = append(result, borderStyle.Render("│")+" "+fixed(line, innerWidth)+" "+borderStyle.Render("│"))
	}
	result = append(result, borderStyle.Render("╰"+strings.Repeat("─", width-2)+"╯"))
	return strings.Join(result, "\n")
}

func targetLines(v ViewData, width, capacity int) ([]string, int, string) {
	if len(v.Rows) == 0 {
		message := "No accepted keys · 6 Keys: pending"
		switch {
		case v.Inventory.Busy && v.Inventory.At.IsZero():
			message = "Loading accepted keys…"
		case v.Inventory.Err != nil && v.Inventory.At.IsZero():
			message = "Inventory unavailable; press r to retry"
		case v.Inventory.At.IsZero():
			message = "Inventory not loaded"
		case v.Query != "":
			message = "No matching IDs; clear search with backspace"
		}
		return []string{muted.Render(clip(message, width))}, 0, "Targets 0/0"
	}
	selectedIndex := 0
	for i, row := range v.Rows {
		if row.Selected {
			selectedIndex = i
			break
		}
	}
	offset := min(max(0, v.Offset), max(0, len(v.Rows)-capacity))
	if selectedIndex < offset {
		offset = selectedIndex
	} else if selectedIndex >= offset+capacity {
		offset = selectedIndex - capacity + 1
	}
	lines := make([]string, 0, len(v.Rows))
	for _, row := range v.Rows {
		text := "  " + clean(row.ID) + " · " + clean(row.Status)
		if row.Selected {
			text = "> " + clean(row.ID) + " · " + clean(row.Status)
			lines = append(lines, selected.Width(width).Render(fixed(clip(text, width), width)))
		} else {
			lines = append(lines, item.Render(clip(text, width)))
		}
	}
	return lines, offset, fmt.Sprintf("Targets %d/%d", selectedIndex+1, len(v.Rows))
}

func overviewLines(v ViewData, width int) []string {
	known := v.Connected + v.NotObserved
	lines := []string{
		"Config: " + v.Context,
		fmt.Sprintf("Accepted keys: %d", v.Inventory.Count),
		fmt.Sprintf("Connected: %d", v.Connected),
		fmt.Sprintf("Not observed: %d", v.NotObserved),
		fmt.Sprintf("Unknown: %d", max(0, v.Inventory.Count-known)),
		"",
		sourceLine("Keys", v.Inventory),
		sourceLine("Presence", v.Presence),
		"",
		"Connection presence is not job success.",
	}
	if v.Presence.Err != nil {
		lines = append(lines, "Presence unavailable; press r to retry")
	}
	return styledLines(lines, width)
}

func detailLines(v ViewData, width int) []string {
	lines := []string{"Select a minion and press enter to read grains."}
	if v.Selected != "" {
		lines = []string{
			"ID: " + v.Selected,
			"Key: accepted",
			"Presence: " + v.SelectedStatus,
			sourceLine("Presence", v.Presence),
			"Hostname: " + summaryGrain(v.Grains, "fqdn", "host"),
			"OS: " + summaryGrain(v.Grains, "os") + " · " + summaryGrain(v.Grains, "osrelease"),
			"OS family: " + summaryGrain(v.Grains, "os_family"),
			"IPv4: " + summaryGrain(v.Grains, "ipv4"),
			"IPv6: " + summaryGrain(v.Grains, "ipv6"),
			"",
			"Minion-reported grains (read job):",
		}
		if !v.Detail.At.IsZero() {
			lines = append(lines, grainLines(v.Grains, v.DetailQuery)...)
		}
		lines = append(lines,
			sourceLine("Grains", v.Detail),
		)
		if v.Detail.At.IsZero() && !v.Detail.Busy && v.Detail.Err == nil {
			lines = append(lines, "Press enter to fetch details")
		}
	}
	return styledLines(lines, width)
}

func summaryGrain(grains map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := grains[key]
		if !ok || value == nil {
			continue
		}
		var text string
		if list, ok := value.([]any); ok {
			parts := make([]string, 0, len(list))
			for _, entry := range list {
				parts = append(parts, fmt.Sprint(entry))
			}
			text = strings.Join(parts, ", ")
		} else {
			text = grainValue(value)
		}
		if text != "" {
			return clip(text, 160)
		}
	}
	return "unavailable"
}

var systemGrains = []string{
	"os", "osrelease", "os_family", "kernel", "kernelrelease", "arch",
	"cpuarch", "num_cpus", "mem_total", "fqdn", "host", "ipv4", "ipv6",
}

const maxGrainValueBytes = 2048

func grainValue(value any) string {
	if value == nil {
		return "null"
	}
	if s, ok := value.(string); ok {
		return s
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "unavailable"
	}
	return string(encoded)
}

func grainLines(grains map[string]any, query string) []string {
	if len(grains) == 0 {
		return []string{"No grains reported."}
	}
	query = strings.ToLower(strings.TrimSpace(query))
	lines := []string{}
	seen := make(map[string]bool, len(systemGrains))
	appendGrain := func(key string) {
		value := grainValue(grains[key])
		if len(value) > maxGrainValueBytes {
			value = strings.ToValidUTF8(value[:maxGrainValueBytes], "") + "… (truncated)"
		}
		parts := strings.Split(value, "\n")
		lines = append(lines, key+": "+parts[0])
		for _, part := range parts[1:] {
			lines = append(lines, "  "+part)
		}
	}
	for _, key := range systemGrains {
		if _, ok := grains[key]; ok {
			seen[key] = true
			if query == "" || strings.Contains(strings.ToLower(key+" "+grainValue(grains[key])), query) {
				if len(lines) == 0 {
					lines = append(lines, "System:")
				}
				appendGrain(key)
			}
		}
	}
	if len(seen) == 0 && query == "" {
		lines = append(lines, "System:")
		lines = append(lines, "No familiar system grains reported.")
	}
	var additional []string
	for key := range grains {
		if !seen[key] {
			additional = append(additional, key)
		}
	}
	sort.Strings(additional)
	additionalShown := false
	for _, key := range additional {
		if query == "" || strings.Contains(strings.ToLower(key+" "+grainValue(grains[key])), query) {
			if !additionalShown {
				if len(lines) > 0 {
					lines = append(lines, "")
				}
				lines = append(lines, "Additional reported grains:")
				additionalShown = true
			}
			appendGrain(key)
		}
	}
	if len(lines) == 0 {
		return []string{"No grains match the Details search; clear it to show all."}
	}
	return lines
}

func styledLines(lines []string, width int) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if width <= 0 {
			result = append(result, "")
			continue
		}
		wrapped := ansi.Hardwrap(ansi.Wordwrap(clean(line), width, " /,="), width, false)
		for part := range strings.SplitSeq(wrapped, "\n") {
			result = append(result, item.Render(part))
		}
	}
	return result
}

func helpLines(width int) []string {
	return styledLines([]string{
		"Navigation", "", "1 Fleet  2 Jobs  4 Events  6 Keys    switch workbench view",
		"t              build a target from accepted minions",
		"s              open selected minion command console",
		"g              graph selected Linux minion resources",
		"h              preview and review selected minion highstate",
		"↑/↓ or j/k    move in Targets; scroll focused pane",
		"pgup/pgdown    move a page in the focused pane",
		"/              search focused Targets IDs or Details grains",
		"enter          inspect / refresh selected grains",
		"tab/shift+tab  switch panels",
		"esc            close search or return to Targets",
		"r              refresh keys and presence",
		"U              review and install an available update",
		"?              close help", "q              quit", "",
		"Grains inspection sends a read-only Salt job.",
	}, width)
}

func Render(v ViewData) string {
	w, h := max(0, v.Width), max(0, v.Height)
	if w == 0 || h == 0 {
		return ""
	}
	hints := []hint{{"h", "highstate"}, {"s", "console"}, {"g", "graph"}, {"t", "targets"}, {"/", "search"}, {"enter", "inspect"}, {"tab", "panel"}, {"r", "refresh"}, {"?", "help"}, {"q", "quit"}}
	showUpdate := v.AvailableUpdate != "" && !v.Searching && !v.DetailSearching && !v.Help
	if showUpdate {
		hints = append(hints, hint{"U", "update"})
	}
	notice := ""
	alert := false
	if v.Searching {
		hints = []hint{{"enter/esc", "close search"}, {"ctrl+c", "quit"}}
		notice = "Type to filter IDs"
	} else if v.DetailSearching {
		hints = []hint{{"enter/esc", "close search"}, {"backspace", "clear"}, {"ctrl+c", "quit"}}
		notice = "Filter grains"
	} else if v.Help {
		hints = []hint{{"↑↓", "scroll help"}, {"esc/?", "return"}, {"ctrl+c", "quit"}}
	} else if v.Inventory.Err != nil {
		hints = []hint{{"r", "retry"}, {"?", "help"}}
		notice, alert = "Keys: "+clean(v.Inventory.Err.Error()), true
	} else if v.Presence.Err != nil {
		hints = []hint{{"r", "retry"}, {"?", "help"}}
		notice, alert = "Presence: "+clean(strings.TrimPrefix(v.Presence.Err.Error(), "presence: ")), true
	}
	if h == 1 {
		return actionBar(w, notice, alert, hints...)
	}
	frame := []string{modeBar(w, "Fleet", v.Context, "")}
	noticeLine := h >= 3 && showUpdate
	if noticeLine {
		label := " " + noticeStyle.Render(v.AvailableUpdate+" available") + "  " + keycap.Render("U") + item.Render(": update")
		frame = append(frame, chromeLine(w, label))
	}
	if h >= 4+len(frame)-1 {
		frame = append(frame, muted.Render(strings.Repeat("─", w)))
	}
	bodyHeight := h - len(frame) - 1
	if bodyHeight > 0 {
		var body string
		details := func(width int, focused bool) string {
			var pinned []string
			title := "Details"
			if v.Selected != "" {
				if v.DetailQuery != "" && !v.Detail.At.IsZero() {
					matches := grainLines(v.Grains, v.DetailQuery)
					if len(matches) == 1 && strings.HasPrefix(matches[0], "No grains match") {
						title = "Details · no matches"
					}
				}
				if bodyHeight >= 5 {
					search := muted
					if v.DetailSearching {
						search = accent
					}
					pinned = []string{search.Render(clip(ansi.Strip(v.DetailSearch), width-4))}
				} else if v.DetailSearching {
					title = ansi.Strip(v.DetailSearch)
				}
			}
			return panel(title, detailLines(v, width-4), v.DetailOffset, width, bodyHeight, focused, pinned)
		}
		if v.Help {
			body = panel("Fleet keys", helpLines(max(0, w-4)), v.HelpOffset, w, bodyHeight, true, nil)
		} else {
			leftWidth := TargetWidth(w)
			leftInner := max(0, leftWidth-4)
			capacity := max(1, bodyHeight-4)
			list, offset, title := targetLines(v, leftInner, capacity)
			search := muted.Render(clip(ansi.Strip(v.Search), leftInner))
			if v.Searching {
				search = accent.Render(clip(ansi.Strip(v.Search), leftInner))
			}
			var pinned []string
			if bodyHeight >= 5 {
				pinned = []string{search}
			} else if v.Searching {
				title = ansi.Strip(v.Search)
			} else if bodyHeight == 3 && len(list) > 0 {
				title = ansi.Strip(list[min(offset, len(list)-1)])
			}
			targets := panel(title, list, offset, leftWidth, bodyHeight, v.Focus == 0, pinned)
			switch {
			case w >= 100:
				middleWidth := (w - 4 - leftWidth) / 2
				rightWidth := w - 4 - leftWidth - middleWidth
				body = lipgloss.JoinHorizontal(lipgloss.Top, targets, "  ",
					panel("Fleet overview", overviewLines(v, middleWidth-4), v.OverviewOffset, middleWidth, bodyHeight, v.Focus == 1, nil), "  ",
					details(rightWidth, v.Focus == 2))
			case w >= 68:
				rightWidth := w - 2 - leftWidth
				if v.Focus == 2 {
					body = lipgloss.JoinHorizontal(lipgloss.Top, targets, "  ", details(rightWidth, true))
				} else {
					body = lipgloss.JoinHorizontal(lipgloss.Top, targets, "  ", panel("Fleet overview", overviewLines(v, rightWidth-4), v.OverviewOffset, rightWidth, bodyHeight, v.Focus == 1, nil))
				}
			default:
				body = targets
				if v.Focus == 1 {
					body = panel("Fleet overview", overviewLines(v, w-4), v.OverviewOffset, w, bodyHeight, true, nil)
				} else if v.Focus == 2 {
					body = details(w, true)
				}
			}
		}
		frame = append(frame, strings.Split(body, "\n")...)
	}
	frame = append(frame, actionBar(w, notice, alert, hints...))
	return strings.Join(frame, "\n")
}
