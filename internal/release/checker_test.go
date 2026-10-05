package release

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"saltrtui/internal/cache"
)

func TestCheckLatestUsesFreshCachedResult(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	store, err := cache.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	userAgent := ""
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		userAgent = request.Header.Get("User-Agent")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"tag_name":"v0.0.2"}`))
	}))
	defer server.Close()

	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	result, err := checkLatest(context.Background(), "v0.0.1", store, server.Client(), func() time.Time { return now }, server.URL)
	if err != nil || !result.Available || result.LatestVersion != "v0.0.2" {
		t.Fatalf("first result = %#v, %v", result, err)
	}
	if userAgent != "saltrtui-update-check" {
		t.Fatalf("user agent = %q", userAgent)
	}
	result, err = checkLatest(context.Background(), "v0.0.1", store, server.Client(), func() time.Time { return now.Add(30 * time.Minute) }, server.URL)
	if err != nil || !result.Available || requests != 1 {
		t.Fatalf("cached result = %#v, %v; requests = %d", result, err, requests)
	}
}

func TestCheckLatestThrottlesFailedRequest(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	store, err := cache.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	result, err := checkLatest(context.Background(), "v0.0.1", store, server.Client(), func() time.Time { return now }, server.URL)
	if err != nil || result.Available {
		t.Fatalf("failed request result = %#v, %v", result, err)
	}
	result, err = checkLatest(context.Background(), "v0.0.1", store, server.Client(), func() time.Time { return now.Add(30 * time.Minute) }, server.URL)
	if err != nil || result.Available || requests != 1 {
		t.Fatalf("throttled result = %#v, %v; requests = %d", result, err, requests)
	}
}

func TestCheckLatestSkipsDevAndRetainsStaleKnownVersion(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	store, err := cache.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	if result, err := CheckLatest(context.Background(), "dev", store); err != nil || result.Available {
		t.Fatalf("dev must never advertise an update: %#v, %v", result, err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"tag_name":"v1.0.0"}`))
	}))
	defer server.Close()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	if _, err := checkLatest(context.Background(), "v0.9.0", store, server.Client(), func() time.Time { return now }, server.URL); err != nil {
		t.Fatal(err)
	}
	server.Close()
	// A later failed attempt retains the previously discovered tag without
	// re-describing it as newly verified.
	failing := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failing.Close()
	result, err := checkLatest(context.Background(), "v0.9.0", store, failing.Client(), func() time.Time { return now.Add(2 * time.Hour) }, failing.URL)
	if err != nil || !result.Available || result.LatestVersion != "v1.0.0" {
		t.Fatalf("stale known version was not retained: %#v, %v", result, err)
	}
}

func TestIsNewer(t *testing.T) {
	for _, test := range []struct {
		candidate string
		current   string
		want      bool
	}{
		{candidate: "v1.0.0", current: "v0.9.9", want: true},
		{candidate: "v1.1.0", current: "v1.0.9", want: true},
		{candidate: "v1.0.1", current: "v1.0.0", want: true},
		{candidate: "v1.0.0", current: "v1.0.0", want: false},
		{candidate: "v1.0.0", current: "v1.0.1", want: false},
		{candidate: "v1.0.1", current: "dev", want: false},
		{candidate: "garbage", current: "v1.0.0", want: false},
	} {
		t.Run(test.candidate+"/"+test.current, func(t *testing.T) {
			if got := isNewer(test.candidate, test.current); got != test.want {
				t.Fatalf("isNewer(%q, %q) = %v, want %v", test.candidate, test.current, got, test.want)
			}
		})
	}
}
