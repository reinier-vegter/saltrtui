package saltcli

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"saltrtui/internal/highstate"
)

const previewStates = `{"node-1":{"pkg_|-baseline_|-base_|-installed":{"result":null,"changes":{"new":"base"},"comment":"Would install"},"service_|-agent_|-agent_|-running":{"result":true,"changes":{},"comment":"Already running"}}}`
const appliedStates = `{"node-1":{"pkg_|-baseline_|-base_|-installed":{"result":true,"changes":{"installed":"base"},"comment":"Installed"},"service_|-agent_|-agent_|-running":{"result":true,"changes":{},"comment":"Already running"}}}`
const failedStates = `{"node-1":{"pkg_|-baseline_|-base_|-installed":{"result":false,"changes":{},"comment":"Package unavailable"}}}`

func TestSelectedHighstateRechecksExactKeyAndUsesFixedArgv(t *testing.T) {
	g := New("/etc/salt")
	var calls [][]string
	g.run = func(_ context.Context, binary string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{binary}, args...))
		if binary == "salt-key" {
			return []byte(`{"minions":["node-1"]}`), nil
		}
		if len(calls) == 2 {
			return []byte(previewStates), nil
		}
		return []byte(appliedStates), nil
	}
	preview, err := g.Preview(context.Background(), "node-1")
	if err != nil || preview.Proposed != 1 || preview.Unchanged != 1 || preview.Failed != 0 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	apply, err := g.Apply(context.Background(), "node-1")
	if err != nil || apply.Changed != 1 || apply.Unchanged != 1 {
		t.Fatalf("apply: %+v %v", apply, err)
	}
	want := [][]string{
		{"salt-key", "-c", "/etc/salt", "--list", "accepted", "--out=json", "--no-color"},
		{"salt", "-c", "/etc/salt", "-L", "node-1", "state.highstate", "test=True", "--timeout=180", "--static", "--out=json", "--no-color"},
		{"salt-key", "-c", "/etc/salt", "--list", "accepted", "--out=json", "--no-color"},
		{"salt", "-c", "/etc/salt", "-L", "node-1", "state.highstate", "test=False", "--timeout=180", "--static", "--out=json", "--no-color"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected highstate argv: %v", calls)
	}
}

func TestHighstateFailuresAndUnsafeTargets(t *testing.T) {
	for _, value := range []string{`{}`, `{"other":{}}`, `{"node-1":{}}`, `{"node-1":[]}`, `{"node-1":{"step":{"result":true,"changes":{},"comment":"ok"}},"other":{}}`,
		`{"node-1":{"step":{"result":"true","changes":{},"comment":"ok"}}}`, `{"node-1":{"step":{"result":true,"changes":null,"comment":"ok"}}}`, `{"node-1":{"step":{"result":true,"changes":{},"comment":null}}}`} {
		if _, err := parseHighstate([]byte(value), "node-1", true); err == nil {
			t.Fatalf("accepted malformed state result %s", value)
		}
	}
	if _, err := parseHighstate([]byte(previewStates), "node-1", false); err == nil {
		t.Fatal("apply accepted test-only state status")
	}
	g := New("")
	g.run = func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("unsafe ID reached subprocess")
		return nil, nil
	}
	for _, id := range []string{"*", "node?", "node[1]", "node,other", "a/b", "a\\b", "-node", "node other", "node\nother", strings.Repeat("x", 256)} {
		if _, err := g.Apply(context.Background(), id); !errors.Is(err, highstate.ErrNotSent) {
			t.Fatalf("unsafe ID %q dispatched: %v", id, err)
		}
	}
}

func TestHighstateMissingAcceptedKeyAndUncertainApply(t *testing.T) {
	g := New("")
	var calls int
	g.run = func(_ context.Context, binary string, _ ...string) ([]byte, error) {
		calls++
		if binary != "salt-key" {
			t.Fatal("state job sent without accepted key")
		}
		return []byte(`{"minions":[]}`), nil
	}
	if _, err := g.Apply(context.Background(), "node-1"); !errors.Is(err, highstate.ErrNotSent) || calls != 1 {
		t.Fatalf("state job not blocked before dispatch: %v calls=%d", err, calls)
	}
	g.run = func(_ context.Context, binary string, _ ...string) ([]byte, error) {
		if binary == "salt-key" {
			return []byte(`{"minions":["node-1"]}`), nil
		}
		return nil, errors.New("timeout")
	}
	if _, err := g.Apply(context.Background(), "node-1"); err == nil || errors.Is(err, highstate.ErrNotSent) || !strings.Contains(err.Error(), "outcome unknown") {
		t.Fatalf("timeout treated as unsent or success: %v", err)
	}
	g.run = func(_ context.Context, binary string, _ ...string) ([]byte, error) {
		if binary == "salt-key" {
			return []byte(`{"minions":["node-1"]}`), nil
		}
		return []byte(failedStates), errors.New("exit status 2")
	}
	if report, err := g.Preview(context.Background(), "node-1"); err != nil || report.Failed != 1 {
		t.Fatalf("failed preview must show state failures, not authorize apply: %+v %v", report, err)
	}
	if report, err := g.Apply(context.Background(), "node-1"); err != nil || report.Failed != 1 {
		t.Fatalf("failed apply must report known failed states: %+v %v", report, err)
	}
}
