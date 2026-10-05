package saltcli

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const jobListFixture = `{"20260925123000000000":{"Function":"state.apply","Target":"web-*","Target-type":"glob","StartTime":"2026, Sep 25 12:30:00"},"20260925124000000000":{"Function":"grains.items","Target":["web-01"],"Target-type":"list","StartTime":"2026, Sep 25 12:40:00"}}`
const jobList3008Fixture = `[{"Function":"runner.state.orchestrate","Arguments":[],"Target":"master","Target-type":"list","User":"root","JID":"20260925123000000000","StartTime":"2026, Sep 25 12:30:00"},{"Function":"state.highstate","Arguments":[],"Target":["web-01"],"Target-type":"list","User":"root","JID":"20260925124000000000","StartTime":"2026, Sep 25 12:40:00"}]`
const jobDetailFixture = `{"jid":"20260925124000000000","Function":"grains.items","Target":["web-01"],"Target-type":"list","StartTime":"2026, Sep 25 12:40:00","Minions":["web-01","web-02"],"Result":{"web-01":{"return":{"nested":[1,false]},"retcode":0,"success":true}}}`

func TestJobsCommandsAndCachedReturn(t *testing.T) {
	g := New("/etc/salt")
	var calls [][]string
	g.run = func(_ context.Context, binary string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{binary}, args...))
		if len(args) > 2 && args[2] == "jobs.list_job" {
			return []byte(jobDetailFixture), nil
		}
		return []byte(jobListFixture), nil
	}
	items, err := g.ListRecentJobs(context.Background())
	if err != nil || len(items) != 2 || items[0].JID != "20260925124000000000" || items[0].Target != "web-01" {
		t.Fatalf("recent jobs: %+v %v", items, err)
	}
	detail, err := g.ReadJob(context.Background(), items[0].JID)
	if err != nil || len(detail.Minions) != 2 || len(detail.Returns) != 1 || detail.Returns["web-01"].Retcode == nil || *detail.Returns["web-01"].Retcode != 0 {
		t.Fatalf("job detail: %+v %v", detail, err)
	}
	var value map[string]any
	if err := json.Unmarshal(detail.Returns["web-01"].Value, &value); err != nil || value["nested"] == nil {
		t.Fatalf("nested return lost: %s %v", detail.Returns["web-01"].Value, err)
	}
	want := [][]string{{"salt-run", "-c", "/etc/salt", "jobs.list_jobs_filter", "50", "--out=json", "--no-color"}, {"salt-run", "-c", "/etc/salt", "jobs.list_job", "20260925124000000000", "--out=json", "--no-color"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("got argv %v; want %v", calls, want)
	}
}

func TestSalt3008ArrayJobs(t *testing.T) {
	g := New("")
	g.run = func(context.Context, string, ...string) ([]byte, error) { return []byte(jobList3008Fixture), nil }
	items, err := g.ListRecentJobs(context.Background())
	if err != nil || len(items) != 2 || items[0].Function != "state.highstate" || items[1].Function != "runner.state.orchestrate" {
		t.Fatalf("Salt 3008 array cache: %+v %v", items, err)
	}
}

func TestJobsRejectInvalidOrExpiredCacheData(t *testing.T) {
	for _, body := range []string{`null`, `[null]`, `[{"Function":"state.highstate","Target":"*","StartTime":"now"}]`,
		`[{"JID":"20260925124000000000","Function":"state.highstate","Target":"*","StartTime":"now"},{"JID":"20260925124000000000","Function":"state.highstate","Target":"*","StartTime":"now"}]`,
		`{"20260925124000000000":{"JID":"20260925125000000000","Function":"state.highstate","Target":"*","StartTime":"now"}}`,
		`{"bad":{"Function":"test.ping","Target":"*","StartTime":"now"}}`, `{"20260925124000000000":{"Target":"*"}}`, `{} {}`} {
		g := New("")
		g.run = func(context.Context, string, ...string) ([]byte, error) { return []byte(body), nil }
		if _, err := g.ListRecentJobs(context.Background()); err == nil {
			t.Fatalf("accepted invalid list %s", body)
		}
	}
	g := New("")
	for _, empty := range []string{`{}`, `[]`} {
		g.run = func(context.Context, string, ...string) ([]byte, error) { return []byte(empty), nil }
		if items, err := g.ListRecentJobs(context.Background()); err != nil || items == nil || len(items) != 0 {
			t.Fatalf("empty cache %s: %v %v", empty, items, err)
		}
	}
	for _, body := range []string{`{}`, `{"jid":"20260925124000000000","Function":"test.ping","Target":"*","StartTime":"now"}`, `{"jid":"20260925124000000000","Function":"test.ping","Target":"*","StartTime":"now","Result":null}`, `{"jid":"20260925124000000000","Function":"test.ping","Target":"*","StartTime":"now","Result":{"web-01":true}}`} {
		g.run = func(context.Context, string, ...string) ([]byte, error) { return []byte(body), nil }
		if _, err := g.ReadJob(context.Background(), "20260925124000000000"); err == nil {
			t.Fatalf("accepted invalid detail %s", body)
		}
	}
	g.run = func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("unsafe jid reached runner")
		return nil, nil
	}
	if _, err := g.ReadJob(context.Background(), "123;whoami"); err == nil {
		t.Fatal("unsafe jid accepted")
	}
	if _, err := g.ReadJob(context.Background(), strings.Repeat("1", 100)); err == nil {
		t.Fatal("unbounded jid accepted")
	}
}
