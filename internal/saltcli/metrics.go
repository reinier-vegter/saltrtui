package saltcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"saltrtui/internal/metrics"
)

func (g *Gateway) metricReturn(ctx context.Context, id, function string) (json.RawMessage, error) {
	if !validID(id) {
		return nil, errors.New("minion ID cannot be targeted safely as a single list item")
	}
	data, err := g.invoke(ctx, "salt", "-L", id, function, "--timeout=5", "--static", "--out=json", "--no-color")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", function, err)
	}
	var response map[string]json.RawMessage
	if err := decode(data, &response); err != nil {
		return nil, fmt.Errorf("%s: %w", function, err)
	}
	if len(response) != 1 || len(response[id]) == 0 {
		return nil, fmt.Errorf("%s: missing or unexpected minion return", function)
	}
	return response[id], nil
}

func (g *Gateway) ProbeKernel(ctx context.Context, id string) (string, error) {
	if !validID(id) {
		return "", errors.New("invalid minion ID")
	}
	data, err := g.invoke(ctx, "salt", "-L", id, "grains.item", "kernel", "--timeout=5", "--static", "--out=json", "--no-color")
	if err != nil {
		return "", fmt.Errorf("kernel probe: %w", err)
	}
	var response map[string]json.RawMessage
	if err := decode(data, &response); err != nil || len(response) != 1 || len(response[id]) == 0 {
		return "", errors.New("kernel probe: invalid minion return")
	}
	var value struct {
		Kernel string `json:"kernel"`
	}
	if err := decode(response[id], &value); err != nil || value.Kernel == "" {
		return "", errors.New("kernel probe: missing kernel")
	}
	return value.Kernel, nil
}

func (g *Gateway) ReadCPU(ctx context.Context, id string) (metrics.Counters, error) {
	data, err := g.metricReturn(ctx, id, "status.cpustats")
	if err != nil {
		return metrics.Counters{}, err
	}
	var response struct {
		CPU map[string]json.RawMessage `json:"cpu"`
	}
	if err := decode(data, &response); err != nil || response.CPU == nil {
		return metrics.Counters{}, errors.New("CPU stats: missing CPU counters")
	}
	fields := []string{"user", "nice", "system", "idle", "iowait", "irq", "softirq", "steal"}
	values := make([]uint64, len(fields))
	for i, name := range fields {
		if len(response.CPU[name]) == 0 || decode(response.CPU[name], &values[i]) != nil {
			return metrics.Counters{}, fmt.Errorf("CPU stats: invalid %s counter", name)
		}
	}
	return metrics.Counters{User: values[0], Nice: values[1], System: values[2], Idle: values[3], IOWait: values[4], IRQ: values[5], SoftIRQ: values[6], Steal: values[7]}, nil
}

func (g *Gateway) ReadMemory(ctx context.Context, id string) (metrics.Memory, error) {
	data, err := g.metricReturn(ctx, id, "status.meminfo")
	if err != nil {
		return metrics.Memory{}, err
	}
	var response map[string]struct {
		Value string `json:"value"`
		Unit  string `json:"unit"`
	}
	if err := decode(data, &response); err != nil || response == nil {
		return metrics.Memory{}, errors.New("memory stats: invalid return")
	}
	read := func(name string) (uint64, error) {
		value, ok := response[name]
		if !ok || value.Unit != "kB" {
			return 0, fmt.Errorf("memory stats: missing or invalid %s in kB", name)
		}
		n, err := strconv.ParseUint(value.Value, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("memory stats: invalid %s value", name)
		}
		return n, nil
	}
	total, err := read("MemTotal")
	if err != nil {
		return metrics.Memory{}, err
	}
	available, err := read("MemAvailable")
	if err != nil {
		return metrics.Memory{}, err
	}
	value := metrics.Memory{Total: total, Available: available}
	if _, err := metrics.Used(value); err != nil {
		return metrics.Memory{}, err
	}
	return value, nil
}
