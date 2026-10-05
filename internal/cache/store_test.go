package cache

import (
	"reflect"
	"testing"
	"time"
)

func TestUpdateCheckRoundTripAndFreshness(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	checkedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	check := UpdateCheck{CheckedAt: checkedAt, LatestVersion: "v0.0.2"}
	if err := store.SaveUpdateCheck(check); err != nil {
		t.Fatal(err)
	}
	if got, err := store.LoadUpdateCheck(); err != nil || !reflect.DeepEqual(got, check) {
		t.Fatalf("round trip = %#v, %v", got, err)
	}
	if !IsUpdateCheckFresh(check, checkedAt.Add(time.Hour)) {
		t.Fatal("check should still be fresh at exactly one hour")
	}
	if IsUpdateCheckFresh(check, checkedAt.Add(time.Hour+time.Nanosecond)) {
		t.Fatal("check should expire just after one hour")
	}
	if IsUpdateCheckFresh(check, checkedAt.Add(-time.Second)) {
		t.Fatal("a future checked-at time must never be treated as fresh")
	}
}

func TestLoadUpdateCheckMissing(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	store, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadUpdateCheck(); err == nil {
		t.Fatal("expected an error for a missing cache entry")
	}
}
