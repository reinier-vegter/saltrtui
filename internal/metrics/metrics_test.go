package metrics

import (
	"math"
	"testing"
	"time"
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
	for i := 0; i < 303; i++ {
		points = Append(points, Point{CPU: float64(i)})
	}
	if len(points) != 300 || points[0].CPU != 3 || points[299].CPU != 302 {
		t.Fatal("series did not evict oldest samples")
	}
}

func TestSecondsCountersAndTimeRetention(t *testing.T) {
	busy, wait, err := Percent(Counters{User: 1.25, Idle: 10.5, IOWait: .25}, Counters{User: 1.75, Idle: 11.75, IOWait: .5})
	if err != nil || busy != 25 || wait != 12.5 {
		t.Fatalf("fractional seconds: %v %v %v", busy, wait, err)
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), -1} {
		if _, _, err := Percent(Counters{}, Counters{User: bad, Idle: 1}); err == nil {
			t.Fatal("invalid counter accepted")
		}
	}
	at := time.Now()
	points := Append([]Point{{At: at.Add(-6 * time.Minute)}, {At: at.Add(-time.Minute)}}, Point{At: at})
	if len(points) != 2 {
		t.Fatal("old history retained")
	}
}
