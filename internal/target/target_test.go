package target

import (
	"reflect"
	"strings"
	"testing"
)

func TestResolveAcceptedKeyCandidates(t *testing.T) {
	keys := []string{"web/a", "db-1", "web-2", "web-1", "web-1", "web-3", "unsafe,id", "wéb-1"}
	for _, test := range []struct {
		mode Mode
		expr string
		want []string
	}{
		{Glob, "web*", []string{"web-1", "web-2", "web-3", "web/a"}},
		{Glob, "web-?", []string{"web-1", "web-2", "web-3"}},
		{Glob, "web-[!2]", []string{"web-1", "web-3"}},
		{List, "web-2, web-1,web-2", []string{"web-1", "web-2"}},
		{Glob, "none*", nil},
		{Glob, "wéb*", []string{"wéb-1"}},
	} {
		got, err := Resolve(test.mode, test.expr, keys)
		if err != nil || !reflect.DeepEqual(got.IDs, test.want) {
			t.Fatalf("%s %q => %v, %v; want %v", test.mode, test.expr, got.IDs, err, test.want)
		}
	}
}

func TestRejectAmbiguousOrMissingListIDs(t *testing.T) {
	for _, expr := range []string{"web-1,", "unknown", "-flag", strings.Repeat("a", 8193)} {
		if _, err := Resolve(List, expr, []string{"web-1", "-flag"}); err == nil {
			t.Fatalf("expected invalid expression: %q", expr)
		}
	}
	if _, err := Resolve(Glob, "abc[", []string{"abc["}); err != nil {
		t.Fatalf("unclosed class should be literal: %v", err)
	}
}

func TestSuggestionsPrefixAndLastListItem(t *testing.T) {
	keys := []string{"web-3", "web-1", "db-1"}
	if got := Suggestions(List, "db-1,we", keys); !reflect.DeepEqual(got, []string{"web-1", "web-3"}) {
		t.Fatalf("list suggestions: %v", got)
	}
	if got := Suggestions(Glob, "web-*", keys); !reflect.DeepEqual(got, []string{"web-1", "web-3"}) {
		t.Fatalf("glob suggestions: %v", got)
	}
	if got := Suggestions(List, "", keys); len(got) != 0 {
		t.Fatalf("empty fragment unexpectedly suggests entire fleet: %v", got)
	}
}
