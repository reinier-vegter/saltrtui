package metrics

import (
	"math"
	"testing"
)

func TestPercentMemoryAndGaps(t *testing.T) {
	a := Counters{User: 10, Idle: 100, IOWait: 5, System: 4, Nice: 2, IRQ: 1, SoftIRQ: 1, Steal: 1}
	b := Counters{User: 30, Idle: 150, IOWait: 15, System: 14, Nice: 2, IRQ: 1, SoftIRQ: 1, Steal: 1}
	busy, wait, err := Percent(a, b)
	if err != nil || math.Abs(busy-100.0/3.0) > .001 || math.Abs(wait-100.0/9.0) > .001 {
		t.Fatalf("delta percent busy=%f wait=%f err=%v", busy, wait, err)
	}
	if _, _, err = Percent(b, a); err == nil {
		t.Fatal("counter reset accepted")
	}
	if _, _, err = Percent(a, a); err == nil {
		t.Fatal("zero interval accepted")
	}
	if got, err := Used(Memory{Total: 1000, Available: 250}); err != nil || got != 75 {
		t.Fatalf("memory: %v %v", got, err)
	}
	if _, err := Used(Memory{Total: 1, Available: 2}); err == nil {
		t.Fatal("bad memory accepted")
	}
	var points []Point
	for i := 0; i < 123; i++ {
		points = Append(points, Point{CPU: float64(i)})
	}
	if len(points) != 120 || points[0].CPU != 3 || points[119].CPU != 122 {
		t.Fatal("series did not evict oldest samples")
	}
}
