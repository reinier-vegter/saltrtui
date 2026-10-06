package saltcli

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"saltrtui/internal/metrics"
)

func TestMetricDiscovery(t *testing.T) {
	for _, tc := range []struct {
		functions string
		want      metrics.Source
	}{
		{`["ps.cpu_times","ps.virtual_memory"]`, metrics.PS},
		{`["ps.cpu_times"]`, metrics.Status},
		{`[]`, metrics.Status},
		{`"unavailable"`, metrics.Status},
	} {
		g := New("/etc/salt")
		g.run = func(_ context.Context, binary string, args ...string) ([]byte, error) {
			want := []string{"-c", "/etc/salt", "-L", "web", "grains.item,sys.list_functions", "kernel", "num_cpus", ",", "ps.cpu_times", "ps.virtual_memory", "--timeout=5", "--static", "--out=json", "--no-color"}
			if binary != "salt" || !reflect.DeepEqual(args, want) {
				t.Fatalf("discovery args: %v", args)
			}
			return []byte(`{"web":{"grains.item":{"kernel":"Linux","num_cpus":4},"sys.list_functions":` + tc.functions + `}}`), nil
		}
		got, err := g.DiscoverMetrics(context.Background(), "web")
		if err != nil || got.Kernel != "Linux" || got.CPUs != 4 || got.Source != tc.want {
			t.Fatalf("capability: %+v %v", got, err)
		}
	}
}

func TestCompoundMetricsBothSources(t *testing.T) {
	for _, source := range []metrics.Source{metrics.Status, metrics.PS} {
		g := New("/etc/salt")
		calls := 0
		g.run = func(_ context.Context, binary string, args ...string) ([]byte, error) {
			calls++
			cpu, mem := "status.cpustats", "status.meminfo"
			cpuData := `{"cpu":{"user":1.25,"nice":2,"system":3,"idle":4,"iowait":5,"irq":6,"softirq":7,"steal":8},"intr":{"many":10}}`
			memData := `{"MemTotal":{"value":"1024","unit":"kB"},"MemAvailable":{"value":"256","unit":"kB"}}`
			if source == metrics.PS {
				cpu, mem = "ps.cpu_times", "ps.virtual_memory"
				cpuData = `{"user":1.25,"nice":2,"system":3,"idle":4,"iowait":5,"irq":6,"softirq":7,"steal":8,"guest":999}`
				memData = `{"total":1048576,"available":262144,"percent":99}`
			}
			want := []string{"-c", "/etc/salt", "-L", "web", cpu + "," + mem + ",status.loadavg", ",,", "--timeout=5", "--static", "--out=json", "--no-color"}
			if binary != "salt" || !reflect.DeepEqual(args, want) {
				t.Fatalf("snapshot args: %v", args)
			}
			return []byte(`{"web":{"` + cpu + `":` + cpuData + `,"` + mem + `":` + memData + `,"status.loadavg":{"1-min":1.2,"5-min":2.3,"15-min":3.4}}}`), nil
		}
		got, err := g.ReadMetrics(context.Background(), "web", source)
		if err != nil || got.CPUErr != nil || got.MemErr != nil || got.LoadErr != nil || calls != 1 || got.CPU.User != 1.25 || got.Memory.Total != 1048576 || got.Load.Fifteen != 3.4 {
			t.Fatalf("snapshot: %+v %v, calls %d", got, err, calls)
		}
	}
}

func TestCompoundMetricsRejectUntrustedReturns(t *testing.T) {
	for _, raw := range []string{`{}`, `{"other":{}}`, `{"web":false}`, `{"web":null}`, `{"web":{},"other":{}}`, `{"web":{}} {}`} {
		g := New("")
		g.run = func(context.Context, string, ...string) ([]byte, error) { return []byte(raw), nil }
		if _, err := g.ReadMetrics(context.Background(), "web", metrics.Status); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	g := New("")
	g.run = func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("unsafe ID invoked")
		return nil, nil
	}
	for _, id := range []string{"web,other", "-web", "web\n"} {
		if _, err := g.ReadMetrics(context.Background(), id, metrics.Status); err == nil {
			t.Fatal("unsafe ID accepted")
		}
		if _, err := g.DiscoverMetrics(context.Background(), id); err == nil {
			t.Fatal("unsafe discovery ID accepted")
		}
	}
}

func TestMetricPartialFailuresAndValidation(t *testing.T) {
	g := New("")
	g.run = func(context.Context, string, ...string) ([]byte, error) {
		return []byte(`{"web":{"ps.cpu_times":false,"ps.virtual_memory":{"total":100,"available":40},"status.loadavg":{"1-min":1,"5-min":2,"15-min":3}}}`), nil
	}
	got, err := g.ReadMetrics(context.Background(), "web", metrics.PS)
	if err != nil || got.CPUErr == nil || got.MemErr != nil || got.LoadErr != nil {
		t.Fatalf("lost partial data: %+v %v", got, err)
	}
	for _, raw := range []string{`null`, `{}`, `{"total":1,"available":2}`, `{"total":0,"available":0}`, `{"total":1}`} {
		if _, err := parseMemory(json.RawMessage(raw), metrics.PS); err == nil {
			t.Fatalf("accepted memory %s", raw)
		}
	}
	for _, raw := range []string{`{"MemTotal":{"value":"1024","unit":"MB"}}`, `{"MemTotal":{"value":"18446744073709551615","unit":"kB"},"MemAvailable":{"value":"0","unit":"kB"}}`} {
		if _, err := parseMemory(json.RawMessage(raw), metrics.Status); err == nil {
			t.Fatalf("accepted status memory %s", raw)
		}
	}
	for _, raw := range []string{`{}`, `null`, `{"1-min":null,"5-min":2,"15-min":3}`, `{"1-min":-1,"5-min":2,"15-min":3}`, `{"1-min":1e999,"5-min":2,"15-min":3}`} {
		if _, err := parseLoad(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted load %s", raw)
		}
	}
	for _, raw := range []string{`null`, `false`, `{}`, `{"user":null,"nice":0,"system":0,"idle":1,"iowait":0,"irq":0,"softirq":0,"steal":0}`, `{"user":-1,"nice":0,"system":0,"idle":1,"iowait":0,"irq":0,"softirq":0,"steal":0}`} {
		if _, err := parseCPU(json.RawMessage(raw), metrics.PS); err == nil {
			t.Fatalf("invalid CPU accepted: %s", raw)
		}
	}
	g.run = func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("timeout") }
	if _, err := g.ReadMetrics(context.Background(), "web", metrics.PS); err == nil {
		t.Fatal("transport error ignored")
	}
}
