package saltcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"saltrtui/internal/keys"
)

const allKeys = `{"minions_pre":["new-2","new-1"],"minions":["web-01"],"minions_rejected":["blocked"],"minions_denied":["duplicate"]}`

func TestKeyInventoryAndDetails(t *testing.T) {
	config := t.TempDir()
	path := filepath.Join(config, "pki", "master", "minions_pre")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "new-1"), []byte("public"), 0600); err != nil {
		t.Fatal(err)
	}
	g := New(config)
	var calls [][]string
	g.run = func(_ context.Context, binary string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{binary}, args...))
		if len(args) > 2 && args[2] == "--finger" {
			return []byte(`{"minions_pre":{"new-1":"AB:CD"}}`), nil
		}
		return []byte(allKeys), nil
	}
	items, err := g.ListKeys(context.Background())
	if err != nil || !reflect.DeepEqual(items, []keys.Key{{ID: "new-1", State: keys.Pending}, {ID: "new-2", State: keys.Pending}, {ID: "web-01", State: keys.Accepted}, {ID: "blocked", State: keys.Rejected}, {ID: "duplicate", State: keys.Denied}}) {
		t.Fatalf("inventory: %v %v", items, err)
	}
	detail, err := g.ReadKey(context.Background(), items[0])
	if err != nil || detail.Fingerprint != "AB:CD" || detail.FileTime.IsZero() {
		t.Fatalf("detail: %+v %v", detail, err)
	}
	if !reflect.DeepEqual(calls[0], []string{"salt-key", "-c", config, "--list", "all", "--out=json", "--no-color"}) || !reflect.DeepEqual(calls[1], []string{"salt-key", "-c", config, "--finger", "new-1", "--out=json", "--no-color"}) {
		t.Fatalf("commands: %v", calls)
	}
	if err := os.Remove(filepath.Join(path, "new-1")); err != nil {
		t.Fatal(err)
	}
	detail, err = g.ReadKey(context.Background(), items[0])
	if err != nil || !detail.FileTime.IsZero() {
		t.Fatalf("missing timestamp should be unknown: %+v %v", detail, err)
	}
	if err := os.Symlink(filepath.Join(path, "elsewhere"), filepath.Join(path, "new-1")); err != nil {
		t.Fatal(err)
	}
	detail, err = g.ReadKey(context.Background(), items[0])
	if err != nil || !detail.FileTime.IsZero() {
		t.Fatalf("symlink must not provide timestamp: %+v %v", detail, err)
	}
	if err := os.Remove(filepath.Join(path, "new-1")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "new-1"), []byte("public"), 0600); err != nil {
		t.Fatal(err)
	}
	g.run = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("fingerprint unavailable")
	}
	detail, err = g.ReadKey(context.Background(), items[0])
	if err == nil || detail.FileTime.IsZero() || detail.Fingerprint != "" {
		t.Fatalf("file timestamp should survive fingerprint failure: %+v %v", detail, err)
	}
}

func TestKeyInventoryFailsClosed(t *testing.T) {
	for _, body := range []string{`{}`, `{"minions_pre":null,"minions":[],"minions_rejected":[],"minions_denied":[]}`,
		`{"minions_pre":["same","same"],"minions":[],"minions_rejected":[],"minions_denied":[]}`,
		`{"minions_pre":[],"minions":[],"minions_rejected":[],"minions_denied":[]} {}`,
		`{'minions_pre': []}`, `{"minions_pre":"new","minions":[],"minions_rejected":[],"minions_denied":[]}`} {
		g := New("")
		g.run = func(context.Context, string, ...string) ([]byte, error) { return []byte(body), nil }
		if _, err := g.ListKeys(context.Background()); err == nil {
			t.Fatalf("accepted %q", body)
		}
	}
}

func TestSingleKeyActionsRecheckAndDoNotExpandGlob(t *testing.T) {
	for _, tc := range []struct {
		action keys.Action
		key    keys.Key
		option string
	}{
		{keys.Accept, keys.Key{ID: "new-1", State: keys.Pending}, "--accept"},
		{keys.Block, keys.Key{ID: "new-1", State: keys.Pending}, "--reject"},
		{keys.Revoke, keys.Key{ID: "web-01", State: keys.Accepted}, "--delete"},
	} {
		g := New("")
		var calls [][]string
		g.run = func(_ context.Context, binary string, args ...string) ([]byte, error) {
			calls = append(calls, append([]string{binary}, args...))
			if len(args) > 0 && args[0] == "--list" {
				return []byte(allKeys), nil
			}
			return nil, nil
		}
		if err := g.ChangeKey(context.Background(), tc.key, tc.action); err != nil {
			t.Fatal(err)
		}
		if len(calls) != 2 || !reflect.DeepEqual(calls[1], []string{"salt-key", tc.option, tc.key.ID, "--yes", "--quiet"}) {
			t.Fatalf("unsafe command: %v", calls)
		}
	}
	g := New("")
	g.run = func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("unexpected subprocess")
		return nil, nil
	}
	for _, id := range []string{"*", "web?", "web[1]", "-x", "../x", "a/b", "a,b", "a\\b", "a\n"} {
		if err := g.ChangeKey(context.Background(), keys.Key{ID: id, State: keys.Pending}, keys.Accept); err == nil {
			t.Fatalf("allowed unsafe ID %q", id)
		}
	}
	if err := g.ChangeKey(context.Background(), keys.Key{ID: "web-01", State: keys.Accepted}, keys.Block); err == nil {
		t.Fatal("blocked accepted key without expected state")
	}
	g.run = func(context.Context, string, ...string) ([]byte, error) { return []byte(allKeys), nil }
	if err := g.ChangeKey(context.Background(), keys.Key{ID: "gone", State: keys.Pending}, keys.Accept); err == nil || !strings.Contains(err.Error(), "not sent") {
		t.Fatalf("missing key dispatched: %v", err)
	}
	g.run = func(context.Context, string, ...string) ([]byte, error) {
		return []byte(`{"minions_pre":[],"minions":["duplicate"],"minions_rejected":[],"minions_denied":["duplicate"]}`), nil
	}
	items, err := g.ListKeys(context.Background())
	if err != nil || len(items) != 2 {
		t.Fatalf("denied duplicate ID must still be visible: %v %v", items, err)
	}
	if err := g.ChangeKey(context.Background(), keys.Key{ID: "duplicate", State: keys.Accepted}, keys.Revoke); err == nil || !strings.Contains(err.Error(), "multiple states") {
		t.Fatalf("ambiguous ID was mutated: %v", err)
	}
	g.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "--list" {
			return []byte(allKeys), nil
		}
		return nil, errors.New("timeout")
	}
	if err := g.ChangeKey(context.Background(), keys.Key{ID: "new-1", State: keys.Pending}, keys.Accept); err == nil || !strings.Contains(err.Error(), "outcome unknown") {
		t.Fatalf("timeout not uncertain: %v", err)
	}
}
