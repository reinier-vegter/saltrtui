package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"saltrtui/internal/jobs"
)

type fakeJobs struct{}

func (fakeJobs) ListRecentJobs(context.Context) ([]jobs.Summary, error) {
	return []jobs.Summary{{JID: "20260925124000000000", Function: "grains.items", Target: "web-01"}}, nil
}
func (fakeJobs) ReadJob(context.Context, string) (jobs.Detail, error) {
	return jobs.Detail{}, errors.New("expired")
}

func TestJobsNavigationPreservesFleetAndIgnoresLateReturns(t *testing.T) {
	m := NewWithJobs(fakeGateway{}, fakeJobs{}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 80, Height: 10})
	m = update(m, keysLoaded{"master-a", 0, []string{"web-01", "web-02"}, time.Now(), nil})
	m = press(m, '/')
	m = typeText(m, "02")
	m = press(m, tea.KeyEscape)
	m = press(m, 'j')
	if m.activeView != 1 || m.jobRequest != 1 || !m.jobList.Busy {
		t.Fatal("Jobs did not start bounded list load")
	}
	first := jobs.Summary{JID: "20260925124000000000", Function: "test.ping", Target: "web-01"}
	second := jobs.Summary{JID: "20260925123000000000", Function: "state.apply", Target: "db-01"}
	m = update(m, jobsLoaded{"master-a", 1, []jobs.Summary{first, second}, time.Now(), nil})
	if m.selectedJob != first.JID {
		t.Fatal("initial job selection missing")
	}
	m = press(m, '/')
	m = typeText(m, "state")
	if m.selectedJob != second.JID || m.search.Value() != "02" {
		t.Fatal("Jobs filter affected Fleet or selected wrong job")
	}
	m = press(m, tea.KeyEscape)
	m = press(m, tea.KeyEnter)
	if m.jobFocus != 1 || !m.jobDetails[second.JID].observation.Busy {
		t.Fatal("job inspection not started")
	}
	m = update(m, jobLoaded{"other-master", 1, second.JID, jobs.Detail{Summary: second}, time.Now(), nil})
	if !m.jobDetails[second.JID].observation.Busy {
		t.Fatal("other context result replaced pending job")
	}
	m = update(m, jobLoaded{"master-a", 1, second.JID, jobs.Detail{Summary: second, Minions: []string{"db-01"}, Returns: map[string]jobs.Return{}}, time.Now(), nil})
	if !strings.Contains(m.View().Content, "Job detail") {
		t.Fatal("job detail view not rendered")
	}
	m = press(m, 'f')
	if m.activeView != 0 || m.selected != "web-02" || m.search.Value() != "02" {
		t.Fatal("Fleet state discarded on return")
	}
	m = press(m, 'j')
	if m.jobSearch.Value() != "state" || m.selectedJob != second.JID || m.jobRequest != 1 {
		t.Fatal("Jobs state or cached list discarded on return")
	}
	m = press(m, 'r')
	if !m.jobDetails[second.JID].observation.Busy {
		t.Fatal("detail retry did not start")
	}
	m = update(m, jobLoaded{"master-a", 2, second.JID, jobs.Detail{}, time.Now(), errors.New("expired")})
	if !strings.Contains(m.View().Content, "stale/error") || m.jobDetails[second.JID].observation.Value.JID != second.JID {
		t.Fatal("failed refresh discarded cached detail")
	}
}

func TestJobsNewestFirstEvenWhenBackendResultsAreUnsorted(t *testing.T) {
	m := NewWithJobs(fakeGateway{}, fakeJobs{}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 12})
	m = press(m, 'j')
	older := jobs.Summary{JID: "20260925123000000000", Function: "state.apply"}
	newer := jobs.Summary{JID: "20260925125000000000", Function: "state.highstate"}
	middle := jobs.Summary{JID: "20260925124000000000", Function: "test.ping"}
	m = update(m, jobsLoaded{context: "master-a", request: 1, items: []jobs.Summary{older, newer, middle}, at: time.Now()})
	items := m.jobsViewData().Items
	if len(items) != 3 || items[0].JID != newer.JID || items[1].JID != middle.JID || items[2].JID != older.JID || m.selectedJob != newer.JID {
		t.Fatalf("recent jobs did not start newest-first: %+v selected %s", items, m.selectedJob)
	}
	m = press(m, '/')
	m = typeText(m, "state")
	items = m.jobsViewData().Items
	if len(items) != 2 || items[0].JID != newer.JID || items[1].JID != older.JID {
		t.Fatalf("filter changed newest-first order: %+v", items)
	}
	m = press(m, tea.KeyEscape)
	m = press(m, 'r')
	m = update(m, jobsLoaded{context: "master-a", request: 2, items: []jobs.Summary{middle, older, newer}, at: time.Now()})
	if m.selectedJob != newer.JID || m.jobsViewData().Items[0].JID != newer.JID {
		t.Fatal("refresh lost newest-first order or selected job")
	}
}
