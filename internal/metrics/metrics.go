// Package metrics normalizes Linux resource counters without terminal dependencies.
package metrics

import (
	"context"
	"errors"
	"math"
	"time"
)

type Counters struct {
	User, Nice, System, Idle, IOWait, IRQ, SoftIRQ, Steal float64
}

type Memory struct{ Total, Available uint64 }

type Source string

const (
	Status Source = "status"
	PS     Source = "ps"
)

type Capability struct {
	Kernel string
	CPUs   int
	Source Source
}

type Load struct{ One, Five, Fifteen float64 }

type Snapshot struct {
	CPU                     Counters
	Memory                  Memory
	Load                    Load
	CPUErr, MemErr, LoadErr error
}

type Gateway interface {
	DiscoverMetrics(context.Context, string) (Capability, error)
	ReadMetrics(context.Context, string, Source) (Snapshot, error)
}

type Point struct {
	BreakBefore        bool
	At                 time.Time
	CPU, IO, Mem       float64
	CPUValid, MemValid bool
	Load               Load
	LoadValid          bool
	Memory             Memory
}

// Percent computes non-idle, non-iowait CPU and separate iowait from consecutive ticks.
func Percent(previous, current Counters) (float64, float64, error) {
	a := []float64{previous.User, previous.Nice, previous.System, previous.Idle, previous.IOWait, previous.IRQ, previous.SoftIRQ, previous.Steal}
	b := []float64{current.User, current.Nice, current.System, current.Idle, current.IOWait, current.IRQ, current.SoftIRQ, current.Steal}
	var total, idle, wait float64
	for i := range a {
		if !Finite(a[i]) || !Finite(b[i]) || a[i] < 0 || b[i] < a[i] {
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
	if total == 0 || !Finite(total) {
		return 0, 0, errors.New("CPU counters have no interval")
	}
	return 100 * (total - idle - wait) / total, 100 * wait / total, nil
}

func Finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func Used(m Memory) (float64, error) {
	if m.Total == 0 || m.Available > m.Total {
		return 0, errors.New("invalid Linux memory counters")
	}
	return 100 * float64(m.Total-m.Available) / float64(m.Total), nil
}

// Append retains a bounded time series, including gaps.
func Append(points []Point, p Point) []Point {
	cutoff := p.At.Add(-5 * time.Minute)
	start := 0
	for start < len(points) && points[start].At.Before(cutoff) {
		start++
	}
	points = points[start:]
	if len(points) >= 300 {
		points = points[len(points)-299:]
	}
	return append(points, p)
}
