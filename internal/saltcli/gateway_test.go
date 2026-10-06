package saltcli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"saltrtui/internal/assignments"
)

func TestFleetCommandsAndResponses(t *testing.T) {
	var calls [][]string
	g := New("/etc/salt")
	g.run = func(_ context.Context, binary string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{binary}, args...))
		switch binary {
		case "salt-key":
			return []byte(`{"minions":["web-02","web-01","web-01"]}`), nil
		case "salt-run":
			return []byte(`["web-02"]`), nil
		case "salt":
			if strings.Contains(strings.Join(args, " "), "state.show_top") {
				return []byte(`{"web-01":{"base":["baseline","apps.web"]},"web-02":{"base":["baseline"]}}`), nil
			}
			return []byte(`{"web-01":{"os":"Ubuntu","osrelease":"24.04","kernel":"Linux","roles":["web","api"],"site":{"rack":13,"active":true}}}`), nil
		}
		return nil, errors.New("unexpected command")
	}
	ids, err := g.ListAcceptedKeys(context.Background())
	if err != nil || !reflect.DeepEqual(ids, []string{"web-01", "web-02"}) {
		t.Fatalf("keys: %v, %v", ids, err)
	}
	presence, err := g.ReadPresence(context.Background())
	if err != nil || !reflect.DeepEqual(presence, []string{"web-02"}) {
		t.Fatalf("presence: %v, %v", presence, err)
	}
	grains, err := g.ReadSelectedGrains(context.Background(), "web-01")
	if err != nil || grains["os"] != "Ubuntu" || grains["kernel"] != "Linux" || grains["site"].(map[string]any)["rack"] != json.Number("13") {
		t.Fatalf("grains: %+v, %v", grains, err)
	}
	top, err := g.ReadStateTop(context.Background(), []string{"web-02", "web-01"})
	if err != nil || !reflect.DeepEqual(top, assignments.Top{"web-01": {"base": {"baseline", "apps.web"}}, "web-02": {"base": {"baseline"}}}) {
		t.Fatalf("state assignments: %+v, %v", top, err)
	}
	want := [][]string{
		{"salt-key", "-c", "/etc/salt", "--list", "accepted", "--out=json", "--no-color"},
		{"salt-run", "-c", "/etc/salt", "manage.present", "--out=json", "--no-color"},
		{"salt", "-c", "/etc/salt", "-L", "web-01", "grains.items", "sanitize=True", "--timeout=5", "--static", "--out=json", "--no-color"},
		{"salt", "-c", "/etc/salt", "-L", "web-01,web-02", "state.show_top", "--timeout=30", "--static", "--out=json", "--no-color"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("command arrays: got %v want %v", calls, want)
	}
}

func TestStateAssignmentsRejectUnexpectedAndInvalidReturns(t *testing.T) {
	for _, body := range []string{
		`null`, `[]`, `{"other":{"base":["baseline"]}}`, `{"web-01":null}`,
		`{"web-01":{"":[]}}`, `{"web-01":{"base":null}}`, `{"web-01":{"base":[""]}}`,
	} {
		g := New("")
		g.run = func(context.Context, string, ...string) ([]byte, error) { return []byte(body), nil }
		if _, err := g.ReadStateTop(context.Background(), []string{"web-01"}); err == nil {
			t.Fatalf("accepted invalid state assignment response %s", body)
		}
	}
	g := New("")
	g.run = func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("empty inventory should not spawn Salt")
		return nil, nil
	}
	if top, err := g.ReadStateTop(context.Background(), nil); err != nil || len(top) != 0 {
		t.Fatalf("empty inventory = %#v, %v", top, err)
	}
}

func TestMalformedAndUnsafeReturns(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"missing accepted group", `{}`},
		{"null accepted group", `{"minions":null}`},
		{"fallback outputter", `{'minions': ['web-01']}`},
		{"trailing document", `{"minions":[]} {"minions":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := New("")
			g.run = func(context.Context, string, ...string) ([]byte, error) { return []byte(tc.body), nil }
			if _, err := g.ListAcceptedKeys(context.Background()); err == nil {
				t.Fatal("invalid output accepted")
			}
		})
	}
	for _, body := range []string{`{}`, `{"other":{"os":"Ubuntu"}}`, `{"web-01":false}`, `{"web-01":null}`, `{"web-01":{"os":"Ubuntu"},"other":{}}`, `{"web-01":{}} {"web-01":{}}`} {
		g := New("")
		g.run = func(context.Context, string, ...string) ([]byte, error) { return []byte(body), nil }
		if _, err := g.ReadSelectedGrains(context.Background(), "web-01"); err == nil {
			t.Fatalf("accepted invalid return %s", body)
		}
	}
	gEmpty := New("")
	gEmpty.run = func(context.Context, string, ...string) ([]byte, error) { return []byte(`{"web-01":{}}`), nil }
	if grains, err := gEmpty.ReadSelectedGrains(context.Background(), "web-01"); err != nil || grains == nil || len(grains) != 0 {
		t.Fatalf("empty grain object should be valid: %v, %v", grains, err)
	}
	g := New("")
	g.run = func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("should not spawn Salt")
		return nil, nil
	}
	if _, err := g.ReadSelectedGrains(context.Background(), "web-01,db-01"); err == nil {
		t.Fatal("comma-separated minion ID was allowed")
	}
}

func TestEmptyInventoryIsNotMissingInventory(t *testing.T) {
	g := New("")
	g.run = func(_ context.Context, binary string, _ ...string) ([]byte, error) {
		if binary == "salt-key" {
			return []byte(`{"minions":[]}`), nil
		}
		return []byte(`[]`), nil
	}
	ids, err := g.ListAcceptedKeys(context.Background())
	if err != nil || len(ids) != 0 {
		t.Fatalf("empty accepted inventory: %v, %v", ids, err)
	}
	ids, err = g.ReadPresence(context.Background())
	if err != nil || len(ids) != 0 {
		t.Fatalf("empty presence: %v, %v", ids, err)
	}
}

func TestRunnerFailureDoesNotLeakDiagnostics(t *testing.T) {
	g := New("")
	g.run = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("command failed")
	}
	_, err := g.ReadPresence(context.Background())
	if err == nil || !strings.Contains(err.Error(), "presence") {
		t.Fatalf("expected contextual error, got %v", err)
	}
}

func TestSalt3008RunnerWarningsAndPermissions(t *testing.T) {
	warnings := "[WARNING ] unclosed Runner <salt.runner.Runner object at 0x7fac03fefe00>; call ``destroy()`` or use as a context manager\n" +
		"unclosed MasterMinion <salt.minion.MasterMinion object at 0x7fac03329400>; call ``destroy()`` or use as a context manager\n"
	if !onlyRunnerCleanupWarnings(warnings) || onlyRunnerCleanupWarnings(warnings+"[ERROR   ] permission denied\n") ||
		onlyRunnerCleanupWarnings("[WARNING ] Failed to open log file, do you have permission to write to /var/log/salt/master?\n") ||
		onlyRunnerCleanupWarnings("[WARNING ] unclosed Runner <salt.runner.Runner object at 0x7fac03fefe00>; call ``destroy()`` or use as a context manager\nmalicious extra\n") {
		t.Fatal("unsafe runner stderr classification")
	}
	// Model the exact process boundary, including zero status and mixed stderr.
	path := filepath.Join(t.TempDir(), "salt-run")
	for _, tc := range []struct {
		name, stderr string
		exit         int
		want         string
	}{
		{"cleanup only", warnings, 0, ""},
		{"mixed diagnostic", warnings + "[ERROR   ] unexpected failure\n", 0, "diagnostics"},
		{"cache permission", "[ERROR   ] prep_jid could not store a jid after 5 tries.\nPermissionError: [Errno 13] Permission denied: '/var/cache/salt/master/jobs/ab/12'\n", 1, "job cache"},
		{"log permission", "[WARNING ] Failed to open log file, do you have permission to write to /var/log/salt/master?\n", 0, "Salt log"},
		{"nonzero despite warnings", warnings, 1, "diagnostics"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The script is test-only; production Salt commands are argv, not shell.
			script := "#!/bin/sh\nprintf '[]'\nprintf '%s' " + shellQuote(tc.stderr) + " >&2\nexit " + string(rune('0'+tc.exit)) + "\n"
			if err := os.WriteFile(path, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			_, err := execute(context.Background(), path)
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("got %v; expected %q", err, tc.want)
			}
		})
	}
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
