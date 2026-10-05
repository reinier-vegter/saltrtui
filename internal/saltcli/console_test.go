package saltcli

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestConsoleGatewayOperations(t *testing.T) {
	g := New("/etc/salt")
	var calls [][]string
	g.run = func(_ context.Context, binary string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{binary}, args...))
		if binary != "salt" {
			t.Fatal(binary)
		}
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "user.info"):
			return []byte(`{"web-01":{"home":"/root"}}`), nil
		case strings.Contains(joined, "cmd=id -un"):
			return []byte(`{"web-01":{"stdout":"root\n","stderr":"","retcode":0}}`), nil
		case strings.Contains(joined, "cmd=pwd"):
			return []byte(`{"web-01":{"stdout":"/srv/www\n","stderr":"","retcode":0}}`), nil
		case strings.Contains(joined, "cmd=tail -c"):
			return []byte(`{"web-01":{"stdout":"#1700000000\necho saved\n","stderr":"","retcode":0}}`), nil
		default:
			return []byte(`{"web-01":{"stdout":"done\n","stderr":"warning","retcode":7}}`), nil
		}
	}
	identity, err := g.Identity(context.Background(), "web-01")
	if err != nil || identity.Account != "root" || identity.Cwd != "/srv/www" || identity.Home != "/root" {
		t.Fatalf("identity: %+v %v", identity, err)
	}
	history, err := g.History(context.Background(), "web-01", "/root")
	if err != nil || !reflect.DeepEqual(history, []string{"echo saved"}) {
		t.Fatalf("saved history: %v %v", history, err)
	}
	result, err := g.Run(context.Background(), "web-01", "echo a=b", "/srv/www")
	if err != nil || result.Retcode != 7 || result.Stderr != "warning" {
		t.Fatalf("command: %+v %v", result, err)
	}
	want := []string{"salt", "-c", "/etc/salt", "-L", "web-01", "cmd.run_all", "cmd=echo a=b", "python_shell=true", "redirect_stderr=False", "rstrip=False", "timeout=30", "shell=/bin/bash", "cwd=/srv/www", "--timeout=40", "--static", "--out=json", "--no-color"}
	if !reflect.DeepEqual(calls[len(calls)-1], want) {
		t.Fatalf("argv: got %v want %v", calls[len(calls)-1], want)
	}
	if !strings.Contains(strings.Join(calls[3], " "), "python_shell=false") {
		t.Fatal("history should use fixed command without a remote shell")
	}
}

func TestConsoleGatewayRejectsMissingMalformedAndMultipleReturns(t *testing.T) {
	for _, raw := range []string{`{}`, `{"other":{"stdout":"","stderr":"","retcode":0}}`, `{"web-01":false}`, `{"web-01":{"stdout":""}}`, `{"web-01":{"stdout":"","stderr":"","retcode":0},"other":{}}`, `{"web-01":{"stdout":"","stderr":"","retcode":0}} {}`} {
		g := New("")
		g.run = func(context.Context, string, ...string) ([]byte, error) { return []byte(raw), nil }
		if _, err := g.Run(context.Background(), "web-01", "pwd", ""); err == nil {
			t.Fatalf("accepted invalid return: %s", raw)
		}
	}
	g := New("")
	g.run = func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("unsafe ID invoked Salt")
		return nil, nil
	}
	if _, err := g.Run(context.Background(), "web-01,db-01", "pwd", ""); err == nil {
		t.Fatal("unsafe ID accepted")
	}
}
