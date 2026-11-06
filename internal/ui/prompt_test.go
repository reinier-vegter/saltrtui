package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestPromptLayoutMeasuresFactsAndWholeControls(t *testing.T) {
	for _, target := range []string{"web-01", strings.Repeat("long-minion-", 20), "東京-🧂", "\x1b[31mweb-01\x1b[0m"} {
		spec := HighstatePrompt(HighstateViewData{ID: target, Context: "/etc/salt"})
		for _, width := range []int{1, 5, 20, 36, 40, 59, 60, 80, 120} {
			for _, height := range []int{1, 3, 8, 11, 12, 18, 40, 120} {
				layout := LayoutPrompt(spec, width, height, min(2, height-1))
				if layout.Height != len(layout.Rows) || layout.BodyHeight+layout.Height+min(2, height-1) != height {
					t.Fatalf("invalid allocation at %dx%d: %+v", width, height, layout)
				}
				for _, row := range layout.Rows {
					if lipgloss.Width(row) != width {
						t.Fatalf("row exceeds budget %d: %q", width, row)
					}
				}
				text := ansi.Strip(strings.Join(layout.Rows, "\n"))
				if layout.FitsConfirmation {
					for _, fact := range append([]string{"▸ " + spec.Title, spec.Consequence}, spec.Facts...) {
						for _, part := range styledLines([]string{ansi.Strip(fact)}, width-4) {
							if !strings.Contains(text, ansi.Strip(part)) {
								t.Fatalf("fit hid fact %q: %s", part, text)
							}
						}
					}
					for _, control := range spec.Controls {
						if !strings.Contains(text, control.key+": "+control.label) {
							t.Fatalf("fit lost complete control: %s", text)
						}
					}
				} else if strings.Contains(text, "y: apply once") {
					t.Fatal("fallback advertised affirmative")
				}
			}
		}
	}
}

func TestAllDecisionsAreBottomFixedAndUncoloredMeaningSurvives(t *testing.T) {
	for _, action := range []string{"accept", "block", "revoke"} {
		v := KeysViewData{Width: 100, Height: 25, Context: "/etc/salt", Confirm: action,
			ConfirmedKey: KeyRow{ID: "pinned-01", State: "pending"}, Selected: KeyRow{ID: "other", State: "accepted"}, DetailOffset: 999}
		assertBottomPrompt(t, RenderKeys(v), KeysPromptLayout(v), "Key: pinned-01", "y: confirm", "n/esc: cancel")
		if strings.Contains(ansi.Strip(strings.Join(KeysPromptLayout(v).Rows, "\n")), "other") {
			t.Fatal("confirmation retargeted to selection")
		}
	}
	v := HighstateViewData{Width: 100, Height: 25, Confirm: true, ID: "web-01", Context: "/etc/salt", Offset: 999}
	assertBottomPrompt(t, RenderHighstate(v), HighstatePromptLayout(v), "Minion: web-01", "y: apply once", "n/esc: cancel")
	u := UpdateViewData{Width: 100, Height: 25, Phase: "confirm", Running: "v1.0.0", Available: "v1.0.1", Current: "/usr/local/bin/saltrtui"}
	assertBottomPrompt(t, RenderSelfUpdate(u), UpdatePromptLayout(u), "Destination: /usr/local/bin/saltrtui", "> Update in place", "esc: back", "sudo required")
}

func assertBottomPrompt(t *testing.T, view string, layout PromptLayout, facts ...string) {
	t.Helper()
	if !layout.FitsConfirmation || !strings.HasSuffix(view, strings.Join(layout.Rows, "\n")) {
		t.Fatal("prompt not bottommost or unexpectedly blocked")
	}
	text := ansi.Strip(strings.Join(layout.Rows, "\n"))
	for _, fact := range facts {
		if !strings.Contains(text, fact) {
			t.Fatalf("non-color decision fact lost %q: %s", fact, text)
		}
	}
	if !strings.Contains(text, "▸") || !strings.HasSuffix(text, "╯") {
		t.Fatal("focus/border requires color")
	}
}

func TestPromptFitsWithNoBodyAndUsesSafeTinyFallback(t *testing.T) {
	spec := HighstatePrompt(HighstateViewData{ID: "web-01", Context: "/etc/salt"})
	full := LayoutPrompt(spec, 80, 80, 2)
	exact := LayoutPrompt(spec, 80, full.Height+2, 2)
	if !exact.FitsConfirmation || exact.BodyHeight != 0 {
		t.Fatal("complete facts cannot fit without body")
	}
	for _, spec := range []PromptSpec{spec, UpdatePrompt(UpdateViewData{})} {
		l := LayoutPrompt(spec, 1, 1, 0)
		if l.FitsConfirmation || (ansi.Strip(l.Rows[0]) != "n" && ansi.Strip(l.Rows[0]) != "q") {
			t.Fatal("tiny screen lost actual safe exit")
		}
		if l := LayoutPrompt(spec, 0, 0, 0); len(l.Rows) != 0 || l.FitsConfirmation {
			t.Fatal("zero dimensions rendered")
		}
	}
}

func TestPromptScopeNeverSilentlyRemovesCharacters(t *testing.T) {
	if promptValue("a\tb") == promptValue(`"a\tb"`) {
		t.Fatal("escaped scope collides with literal quotes/backslash")
	}
	for _, value := range []string{"web\n01", "\x1b[31mweb-01\x1b[0m", "/opt/\tapp", "web\u200b01"} {
		spec := HighstatePrompt(HighstateViewData{ID: value, Context: value})
		layout := LayoutPrompt(spec, 120, 30, 2)
		if !layout.FitsConfirmation {
			t.Fatal("escaped scope cannot be reviewed")
		}
		text := ansi.Strip(strings.Join(layout.Rows, "\n"))
		if !strings.Contains(text, promptValue(value)) || strings.Contains(text, value) {
			t.Fatalf("scope not escaped exactly: %q", text)
		}
		key := KeysPrompt(KeysViewData{ConfirmedKey: KeyRow{ID: value, State: "pending"}, Confirm: "accept", Context: value})
		update := UpdatePrompt(UpdateViewData{Current: value})
		if !strings.Contains(strings.Join(key.Facts, "\n"), promptValue(value)) || !strings.Contains(strings.Join(update.Facts, "\n"), promptValue(value)) {
			t.Fatal("key/update scope sanitized silently")
		}
	}
}
