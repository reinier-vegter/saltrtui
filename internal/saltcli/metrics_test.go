package saltcli

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestLinuxMetricsGateway(t *testing.T) {
	g := New("/etc/salt")
	var calls [][]string
	g.run = func(_ context.Context, binary string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{binary}, args...))
		switch {
		case strings.Contains(strings.Join(args, " "), "grains.item"):
			return []byte(`{"web":{"kernel":"Linux"}}`), nil
		case strings.Contains(strings.Join(args, " "), "cpustats"):
			return []byte(`{"web":{"cpu":{"user":1,"nice":2,"system":3,"idle":4,"iowait":5,"irq":6,"softirq":7,"steal":8},"intr":{"many":10}}}`), nil
		default:
			return []byte(`{"web":{"MemTotal":{"value":"1024","unit":"kB"},"MemAvailable":{"value":"256","unit":"kB"}}}`), nil
		}
	}
	if kernel, err := g.ProbeKernel(context.Background(), "web"); err != nil || kernel != "Linux" {
		t.Fatalf("kernel %q %v", kernel, err)
	}
	if cpu, err := g.ReadCPU(context.Background(), "web"); err != nil || cpu.IOWait != 5 || cpu.Steal != 8 {
		t.Fatalf("CPU %+v %v", cpu, err)
	}
	if mem, err := g.ReadMemory(context.Background(), "web"); err != nil || mem.Total != 1024 || mem.Available != 256 {
		t.Fatalf("mem %+v %v", mem, err)
	}
	want := []string{"salt", "-c", "/etc/salt", "-L", "web", "status.cpustats", "--timeout=5", "--static", "--out=json", "--no-color"}
	if !reflect.DeepEqual(calls[1], want) {
		t.Fatalf("CPU argv: %v want %v", calls[1], want)
	}
}

func TestMetricsRejectUntrustedReturns(t *testing.T) {
	for _, raw := range []string{`{}`, `{"other":{"cpu":{}}}`, `{"web":false}`, `{"web":{"cpu":{"user":1}}}`, `{"web":{"cpu":{"user":1,"nice":2,"system":3,"idle":4,"iowait":5,"irq":6,"softirq":7,"steal":8}},"other":{}}`, `{"web":{}} {}`} {
		g := New("")
		g.run = func(context.Context, string, ...string) ([]byte, error) { return []byte(raw), nil }
		if _, err := g.ReadCPU(context.Background(), "web"); err == nil {
			t.Fatalf("accepted CPU response %s", raw)
		}
	}
	for _, raw := range []string{`{"web":{"MemTotal":{"value":"1024","unit":"kB"}}}`, `{"web":{"MemTotal":{"value":"1024","unit":"MB"},"MemAvailable":{"value":"2","unit":"kB"}}}`, `{"web":{"MemTotal":{"value":"1","unit":"kB"},"MemAvailable":{"value":"2","unit":"kB"}}}`} {
		g := New("")
		g.run = func(context.Context, string, ...string) ([]byte, error) { return []byte(raw), nil }
		if _, err := g.ReadMemory(context.Background(), "web"); err == nil {
			t.Fatalf("accepted memory response %s", raw)
		}
	}
	g := New("")
	g.run = func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("unsafe ID invoked Salt")
		return nil, nil
	}
	if _, err := g.ReadCPU(context.Background(), "web,other"); err == nil {
		t.Fatal("unsafe ID accepted")
	}
}
