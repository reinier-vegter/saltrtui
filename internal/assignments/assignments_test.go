package assignments

import (
	"reflect"
	"testing"
)

func TestRowsGroupsAndSortsResponders(t *testing.T) {
	top := Top{
		"node-b": {"base": {"baseline", "apps.logstash"}},
		"node-a": {"base": {"baseline"}, "dev": {"apps.test"}},
	}
	got := Rows(top)
	want := []Row{
		{Environment: "base", State: "apps.logstash", Minions: []string{"node-b"}},
		{Environment: "base", State: "baseline", Minions: []string{"node-a", "node-b"}},
		{Environment: "dev", State: "apps.test", Minions: []string{"node-a"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Rows() = %#v, want %#v", got, want)
	}
}

func TestTreeBuildsStableEnvironmentStateMinionHierarchy(t *testing.T) {
	top := Top{"node-b": {"base": {"apps.web"}}, "node-a": {"base": {"apps.web"}, "dev": {"apps.test"}}}
	got := Tree(top)
	want := []Node{
		{ID: EnvironmentID("base"), Environment: "base", Kind: EnvironmentNode},
		{ID: StateID("base", "apps.web"), ParentID: EnvironmentID("base"), Environment: "base", State: "apps.web", Kind: StateNode},
		{ID: MinionID("base", "apps.web", "node-a"), ParentID: StateID("base", "apps.web"), Environment: "base", State: "apps.web", Minion: "node-a", Kind: MinionNode},
		{ID: MinionID("base", "apps.web", "node-b"), ParentID: StateID("base", "apps.web"), Environment: "base", State: "apps.web", Minion: "node-b", Kind: MinionNode},
		{ID: EnvironmentID("dev"), Environment: "dev", Kind: EnvironmentNode},
		{ID: StateID("dev", "apps.test"), ParentID: EnvironmentID("dev"), Environment: "dev", State: "apps.test", Kind: StateNode},
		{ID: MinionID("dev", "apps.test", "node-a"), ParentID: StateID("dev", "apps.test"), Environment: "dev", State: "apps.test", Minion: "node-a", Kind: MinionNode},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Tree() = %#v, want %#v", got, want)
	}
}
