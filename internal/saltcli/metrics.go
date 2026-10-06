package saltcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"

	"saltrtui/internal/metrics"
)

// compoundMetrics keeps one exact target and one publish per snapshot. Empty
// argument lists need explicit comma placeholders in Salt's compound CLI parser.
func (g *Gateway) compoundMetrics(ctx context.Context, id, functions string, args ...string) (map[string]json.RawMessage, error) {
	if !validID(id) {
		return nil, errors.New("invalid minion ID")
	}
	argv := append([]string{"-L", id, functions}, args...)
	argv = append(argv, "--timeout=5", "--static", "--out=json", "--no-color")
	data, err := g.invoke(ctx, "salt", argv...)
	if err != nil {
		return nil, err
	}
	var outer map[string]json.RawMessage
	if err := decode(data, &outer); err != nil {
		return nil, err
	}
	if len(outer) != 1 || len(outer[id]) == 0 {
		return nil, errors.New("metrics: missing or unexpected minion return")
	}
	var results map[string]json.RawMessage
	if err := decode(outer[id], &results); err != nil || results == nil {
		return nil, errors.New("metrics: invalid compound return")
	}
	return results, nil
}

func (g *Gateway) DiscoverMetrics(ctx context.Context, id string) (metrics.Capability, error) {
	results, err := g.compoundMetrics(ctx, id, "grains.item,sys.list_functions", "kernel", "num_cpus", ",", "ps.cpu_times", "ps.virtual_memory")
	if err != nil {
		return metrics.Capability{}, err
	}
	var grains struct {
		Kernel string `json:"kernel"`
		CPUs   int    `json:"num_cpus"`
	}
	if decode(results["grains.item"], &grains) != nil || grains.Kernel == "" {
		return metrics.Capability{}, errors.New("metrics: missing kernel")
	}
	capability := metrics.Capability{Kernel: grains.Kernel, CPUs: max(0, grains.CPUs), Source: metrics.Status}
	var functions []string
	if decode(results["sys.list_functions"], &functions) == nil {
		cpu, memory := false, false
		for _, function := range functions {
			cpu = cpu || function == "ps.cpu_times"
			memory = memory || function == "ps.virtual_memory"
		}
		if cpu && memory {
			capability.Source = metrics.PS
		}
	}
	return capability, nil
}

func parseCPU(data json.RawMessage, source metrics.Source) (metrics.Counters, error) {
	if source == metrics.Status {
		var wrapper struct {
			CPU json.RawMessage `json:"cpu"`
		}
		if decode(data, &wrapper) != nil {
			return metrics.Counters{}, errors.New("CPU: invalid stats")
		}
		data = wrapper.CPU
	}
	var fields map[string]*float64
	if decode(data, &fields) != nil || fields == nil {
		return metrics.Counters{}, errors.New("CPU: missing counters")
	}
	values := make([]float64, 8)
	for i, name := range []string{"user", "nice", "system", "idle", "iowait", "irq", "softirq", "steal"} {
		v, ok := fields[name]
		if !ok || v == nil || !metrics.Finite(*v) || *v < 0 {
			return metrics.Counters{}, fmt.Errorf("CPU: invalid %s", name)
		}
		values[i] = *v
	}
	return metrics.Counters{User: values[0], Nice: values[1], System: values[2], Idle: values[3], IOWait: values[4], IRQ: values[5], SoftIRQ: values[6], Steal: values[7]}, nil
}

func parseMemory(data json.RawMessage, source metrics.Source) (metrics.Memory, error) {
	var memory metrics.Memory
	if source == metrics.PS {
		var value struct {
			Total     uint64  `json:"total"`
			Available *uint64 `json:"available"`
		}
		if decode(data, &value) != nil || value.Available == nil {
			return memory, errors.New("memory: missing counters")
		}
		memory = metrics.Memory{Total: value.Total, Available: *value.Available}
	} else {
		var fields map[string]struct {
			Value string `json:"value"`
			Unit  string `json:"unit"`
		}
		if decode(data, &fields) != nil {
			return memory, errors.New("memory: invalid stats")
		}
		values := make([]uint64, 2)
		for i, name := range []string{"MemTotal", "MemAvailable"} {
			field, ok := fields[name]
			value, err := strconv.ParseUint(field.Value, 10, 64)
			if !ok || field.Unit != "kB" || err != nil || value > math.MaxUint64/1024 {
				return memory, fmt.Errorf("memory: invalid %s in kB", name)
			}
			values[i] = value * 1024
		}
		memory = metrics.Memory{Total: values[0], Available: values[1]}
	}
	_, err := metrics.Used(memory)
	return memory, err
}

func parseLoad(data json.RawMessage) (metrics.Load, error) {
	var fields map[string]*float64
	if decode(data, &fields) != nil {
		return metrics.Load{}, errors.New("load: invalid stats")
	}
	values := make([]float64, 3)
	for i, name := range []string{"1-min", "5-min", "15-min"} {
		v, ok := fields[name]
		if !ok || v == nil || !metrics.Finite(*v) || *v < 0 {
			return metrics.Load{}, fmt.Errorf("load: invalid %s", name)
		}
		values[i] = *v
	}
	return metrics.Load{One: values[0], Five: values[1], Fifteen: values[2]}, nil
}

func (g *Gateway) ReadMetrics(ctx context.Context, id string, source metrics.Source) (metrics.Snapshot, error) {
	var snapshot metrics.Snapshot
	cpu, memory := "status.cpustats", "status.meminfo"
	if source == metrics.PS {
		cpu, memory = "ps.cpu_times", "ps.virtual_memory"
	} else if source != metrics.Status {
		return snapshot, errors.New("metrics: unknown source")
	}
	results, err := g.compoundMetrics(ctx, id, cpu+","+memory+",status.loadavg", ",,")
	if err != nil {
		return snapshot, err
	}
	snapshot.CPU, snapshot.CPUErr = parseCPU(results[cpu], source)
	snapshot.Memory, snapshot.MemErr = parseMemory(results[memory], source)
	snapshot.Load, snapshot.LoadErr = parseLoad(results["status.loadavg"])
	return snapshot, nil
}
