package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestAssignmentsFilterDoesNotRenderNestedANSISequencesLiterally(t *testing.T) {
	v := AssignmentsViewData{
		Width: 100, Height: 16, Available: true, Filtering: true,
		Filter: "\x1b[37m/ \x1b[38;5;240mfilter assignments or minions\x1b[m",
		Nodes:  []AssignmentNode{{ID: "environment\x00base", Environment: "base", Kind: AssignmentEnvironment, Expanded: true, Selected: true}},
	}
	view := RenderAssignments(v)
	if strings.Contains(ansi.Strip(view), "[38;5;240m") || strings.Contains(view, "\x1b[37m/ ") {
		t.Fatalf("nested input ANSI sequence leaked into Assignments panel:\n%s", ansi.Strip(view))
	}
	if !strings.Contains(ansi.Strip(view), "filter assignments or minions") {
		t.Fatalf("filter placeholder was lost:\n%s", ansi.Strip(view))
	}
}
