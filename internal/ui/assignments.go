package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type AssignmentNodeKind uint8

const (
	AssignmentEnvironment AssignmentNodeKind = iota
	AssignmentState
	AssignmentMinion
)

type AssignmentNode struct {
	ID, Environment, State, Minion string
	Kind                           AssignmentNodeKind
	Expanded, Selected             bool
}

type AssignmentsViewData struct {
	Context, Selected          string
	Width, Height              int
	Focus                      int
	Offset, DetailOffset       int
	Scope                      []string
	Nodes                      []AssignmentNode
	Result                     Source
	Filter, Query              string
	Help, Filtering, Available bool
}

func assignmentLines(v AssignmentsViewData, width int) []string {
	if !v.Available {
		return styledLines([]string{"Assignments unavailable; no Salt gateway configured."}, width)
	}
	if v.Result.At.IsZero() && !v.Result.Busy && v.Result.Err == nil {
		return styledLines([]string{"Waiting for an accepted-key inventory before fetching assignments."}, width)
	}
	if len(v.Nodes) == 0 {
		message := "No assignments returned by responding minions."
		if len(v.Scope) == 0 {
			message = "No accepted minions to inspect."
		} else if v.Query != "" {
			message = "No assignments or minions match the filter."
		}
		return styledLines([]string{message}, width)
	}
	lines := make([]string, 0, len(v.Nodes))
	for _, node := range v.Nodes {
		var text string
		switch node.Kind {
		case AssignmentEnvironment:
			marker := "▸"
			if node.Expanded || v.Query != "" {
				marker = "▾"
			}
			text = marker + " " + node.Environment
		case AssignmentState:
			marker := "▸"
			if node.Expanded || v.Query != "" {
				marker = "▾"
			}
			text = "  " + marker + " " + node.State
		case AssignmentMinion:
			text = "      " + node.Minion
		}
		if node.Selected {
			text = "> " + strings.TrimPrefix(text, " ")
			lines = append(lines, selected.Width(width).Render(fixed(clip(text, width), width)))
		} else {
			lines = append(lines, item.Render(clip(text, width)))
		}
	}
	return lines
}

func selectedAssignment(v AssignmentsViewData) *AssignmentNode {
	for i := range v.Nodes {
		if v.Nodes[i].Selected {
			return &v.Nodes[i]
		}
	}
	return nil
}

func assignmentDetailLines(v AssignmentsViewData, width int) []string {
	node := selectedAssignment(v)
	if node == nil {
		return styledLines([]string{"Select a tree entry to inspect its assignment context."}, width)
	}
	lines := []string{"Highstate top-file selection", "Not applied-state status.", ""}
	switch node.Kind {
	case AssignmentEnvironment:
		lines = append(lines, "Environment: "+node.Environment, "Select an SLS assignment to inspect its responding minions.")
	case AssignmentState:
		lines = append(lines, "Environment: "+node.Environment, "SLS: "+node.State, "Expand this assignment to browse responding minions.")
	case AssignmentMinion:
		lines = append(lines, "Minion: "+node.Minion, "Environment: "+node.Environment, "SLS: "+node.State,
			"This return does not prove that the state was applied.")
	}
	return styledLines(lines, width)
}

func assignmentsHelpLines(width int) []string {
	return styledLines([]string{
		"Assignments", "",
		"Browse responding minions by highstate environment and assigned SLS.",
		"An SLS is not automatically a role, installed service, or applied state.", "",
		"r              refresh assignments (a read-only Salt job)",
		"enter/space/right expand an environment or SLS", "left          collapse an entry or select its parent",
		"up/down       move in the tree or scroll Details", "tab           switch panes",
		"/              filter environments, SLS names, or minion IDs locally", "?              close help",
	}, width)
}

func assignmentStatus(v AssignmentsViewData) string {
	if v.Result.Busy {
		return fmt.Sprintf("Fetching assignments for %d accepted minions…", len(v.Scope))
	}
	if v.Result.Err != nil {
		return clean(v.Result.Err.Error())
	}
	if !v.Result.At.IsZero() {
		return fmt.Sprintf("%d/%d minions responded · %s", v.Result.Count, len(v.Scope), stamp(v.Result))
	}
	return "Assignments are fetched when this tab opens."
}

func AssignmentsScrollLimit(v AssignmentsViewData, pane int) int {
	if v.Width < 6 || v.Height < 6 {
		return 0
	}
	width := v.Width - 4
	if v.Width >= 68 {
		width = (v.Width - 2) / 2
		if pane == 1 {
			width = v.Width - 2 - width
		}
	}
	lines := assignmentLines(v, width-4)
	if pane == 1 {
		lines = assignmentDetailLines(v, width-4)
	}
	return max(0, len(lines)-PageRows(v.Height, pane, false))
}

func RenderAssignments(v AssignmentsViewData) string {
	w, h := max(0, v.Width), max(0, v.Height)
	if w == 0 || h == 0 {
		return ""
	}
	hints := []hint{{"r", "refresh"}, {"↵/space", "expand"}, {"←", "collapse"}, {"/", "filter"}, {"tab", "pane"}, {"?", "help"}, {"q", "quit"}}
	if v.Filtering {
		hints = []hint{{"enter", "finish filter"}, {"esc", "clear"}, {"ctrl+c", "quit"}}
	} else if v.Help {
		hints = []hint{{"esc/?", "return"}, {"q", "quit"}}
	} else if v.Result.Busy {
		hints = []hint{{"↑↓", "browse prior tree"}, {"/", "filter"}, {"?", "help"}, {"q", "quit"}}
	}
	notice := assignmentStatus(v)
	alert := v.Result.Err != nil
	if h == 1 {
		return actionBar(w, notice, alert, hints...)
	}
	frame := modeFrame(w, h, "Assignments", v.Context, "")
	bodyHeight := h - len(frame) - 1
	if bodyHeight > 0 {
		var body string
		if v.Help {
			body = panel("Assignments help", assignmentsHelpLines(w-4), 0, w, bodyHeight, true, nil)
		} else {
			listTitle := "Assignments"
			// textinput.View() includes its own ANSI styles. Strip those before
			// embedding it in another styled/pinned panel line; otherwise the
			// escape bytes are rendered literally in some terminals.
			filterText := clip(ansi.Strip(strings.TrimPrefix(v.Filter, "/ ")), max(0, w-8))
			filter := muted.Render(filterText)
			if v.Filtering {
				filter = accent.Render(filterText)
			}
			if w >= 68 {
				left := (w - 2) / 2
				right := w - 2 - left
				body = lipgloss.JoinHorizontal(lipgloss.Top,
					panel(listTitle, assignmentLines(v, left-4), v.Offset, left, bodyHeight, v.Focus == 0, []string{filter}), "  ",
					panel("Details", assignmentDetailLines(v, right-4), v.DetailOffset, right, bodyHeight, v.Focus == 1, nil))
			} else if v.Focus == 1 {
				body = panel("Details", assignmentDetailLines(v, w-4), v.DetailOffset, w, bodyHeight, true, nil)
			} else {
				body = panel(listTitle, assignmentLines(v, w-4), v.Offset, w, bodyHeight, true, []string{filter})
			}
		}
		frame = append(frame, strings.Split(body, "\n")...)
	}
	frame = append(frame, actionBar(w, notice, alert, hints...))
	return strings.Join(frame, "\n")
}
