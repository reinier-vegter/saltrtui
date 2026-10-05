// Package metrics normalizes Linux resource counters without terminal dependencies.
package metrics

import (
	"context"
	"errors"
	"time"
)

type Counters struct {
	User, Nice, System, Idle, IOWait, IRQ, SoftIRQ, Steal uint64
}

type Memory struct{ Total, Available uint64 }

type Gateway interface {
	ProbeKernel(context.Context, string) (string, error)
	ReadCPU(context.Context, string) (Counters, error)
	ReadMemory(context.Context, string) (Memory, error)
}

type Point struct {
	At                 time.Time
	CPU, IO, Mem       float64
	CPUValid, MemValid bool
}

// Percent computes non-idle, non-iowait CPU and separate iowait from consecutive ticks.
func Percent(previous, current Counters) (float64, float64, error) {
	a := []uint64{previous.User, previous.Nice, previous.System, previous.Idle, previous.IOWait, previous.IRQ, previous.SoftIRQ, previous.Steal}
	b := []uint64{current.User, current.Nice, current.System, current.Idle, current.IOWait, current.IRQ, current.SoftIRQ, current.Steal}
	var total, idle, wait uint64
	for i := range a {
		if b[i] < a[i] || total > ^uint64(0)-(b[i]-a[i]) {
			return 0, 0, errors.New("CPU counters reset or overflowed")
		}
		delta := b[i] - a[i]
		total += delta
		if i == 3 {
			idle = delta
		}
		if i == 4 {
			wait = delta
		}
	}
	if total == 0 {
		return 0, 0, errors.New("CPU counters have no interval")
	}
	return 100 * float64(total-idle-wait) / float64(total), 100 * float64(wait) / float64(total), nil
}

func Used(m Memory) (float64, error) {
	if m.Total == 0 || m.Available > m.Total {
		return 0, errors.New("invalid Linux memory counters")
	}
	return 100 * float64(m.Total-m.Available) / float64(m.Total), nil
}

// Append retains a bounded time series, including gaps.
func Append(points []Point, p Point) []Point {
	if len(points) >= 120 {
		copy(points, points[1:])
		points[len(points)-1] = p
		return points
	}
	return append(points, p)
}
