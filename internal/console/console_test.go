package console

import (
	"strings"
	"testing"
)

func TestSavedBashHistoryAndNavigation(t *testing.T) {
	saved := ParseHistory("#1700000000\necho first\n#1700000001\nif true; then\necho second\nfi\n")
	if len(saved) != 2 || saved[1] != "if true; then\necho second\nfi" {
		t.Fatalf("timestamped multiline history: %#v", saved)
	}
	h := HistoryState{Saved: saved}
	h.Add("echo local")
	if got := h.Up("draft"); got != "echo local" {
		t.Fatal(got)
	}
	if got := h.Up("echo local"); got != saved[1] {
		t.Fatal(got)
	}
	if got := h.Down(saved[1]); got != "echo local" {
		t.Fatal(got)
	}
	if got := h.Down("echo local"); got != "draft" {
		t.Fatal(got)
	}
	if index, value := h.Reverse("echo", h.Len()); index != 2 || value != "echo local" {
		t.Fatalf("reverse search newest: %d %q", index, value)
	}
	if index, value := h.Reverse("echo", 2); index != 1 || !strings.Contains(value, "echo second") {
		t.Fatalf("reverse search older: %d %q", index, value)
	}
}

func TestHistoryBound(t *testing.T) {
	entries := ParseHistory(strings.Repeat("pwd\n", MaxHistoryEntries+20))
	if len(entries) != MaxHistoryEntries {
		t.Fatalf("saved history unbounded: %d", len(entries))
	}
}
