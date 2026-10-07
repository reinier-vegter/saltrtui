package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"saltrtui/internal/events"
)

type fakeEventGateway struct{ stream chan events.Update }

func (f fakeEventGateway) Subscribe(context.Context) (<-chan events.Update, error) {
	return f.stream, nil
}

func TestEventSubscriptionInspectionAndLifecycle(t *testing.T) {
	stream := make(chan events.Update, 4)
	m := NewWithEvents(fakeGateway{}, nil, nil, fakeEventGateway{stream}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 10})
	var started eventsStarted
	for _, cmd := range m.Init()().(tea.BatchMsg) {
		if msg, ok := cmd().(eventsStarted); ok {
			started = msg
		}
	}
	if started.err != nil || started.stream != stream {
		t.Fatalf("app initialization did not start the subscription: %+v", started)
	}
	next, wait := m.Update(started)
	m = next.(Model)
	if wait == nil {
		t.Fatal("events subscription did not schedule an update")
	}
	stream <- events.Update{Record: events.Record{Tag: "salt/job/1/new", JID: "1", Observed: time.Now(), Data: json.RawMessage(`{"fun":"test.ping"}`)}}
	next, wait = m.Update(wait())
	m = next.(Model)
	if len(m.event.buffer.Records) != 1 || m.event.selected != 0 || wait == nil || m.activeView != 0 {
		t.Fatal("first event was not captured in Fleet or next read not scheduled")
	}
	next, navigation := m.Update(tea.KeyPressMsg{Code: 'e'})
	m = next.(Model)
	if m.activeView != 4 || navigation != nil || len(m.event.buffer.Records) != 1 {
		t.Fatal("opening Events restarted the listener or cleared history")
	}
	m = press(m, tea.KeyEnter)
	if !m.event.paused || m.event.inspect != 0 || m.event.focus != 1 {
		t.Fatal("inspection should hold the selected payload during live updates")
	}
	m = update(m, eventNext{context: "master-a", request: m.eventGeneration, ok: true,
		update: events.Update{Record: events.Record{Tag: "salt/key", Observed: time.Now(), Data: json.RawMessage(`{}`)}, Dropped: 2}})
	if m.event.inspect != 0 || m.event.unseen != 1 || m.event.streamDropped != 2 {
		t.Fatal("new event disturbed inspected payload or hid stream gaps")
	}
	m = press(m, 'c')
	if len(m.event.buffer.Records) != 0 || m.event.streamDropped != 2 {
		t.Fatal("clear should retain dropped-event diagnostics")
	}
	m = press(m, tea.KeyEscape) // Leave details focus.
	m = press(m, tea.KeyEscape) // Leave Events.
	if m.activeView != 0 || m.event.cancel == nil {
		t.Fatal("leaving Events stopped the background listener")
	}
	m = update(m, eventNext{context: "master-a", request: m.eventGeneration, ok: true,
		update: events.Update{Record: events.Record{Tag: "salt/minion/node/start", Data: json.RawMessage(`{}`)}}})
	if len(m.event.buffer.Records) != 1 {
		t.Fatal("event from inactive view was not buffered")
	}
	m = press(m, 'e')
	if len(m.event.buffer.Records) != 1 || m.event.buffer.Records[0].Tag != "salt/minion/node/start" {
		t.Fatal("reopening Events lost background events")
	}
	m = update(m, eventNext{context: "master-a", request: m.eventGeneration, ok: true,
		update: events.Update{Err: errors.New("disconnected"), Dropped: 3}})
	if m.event.streamDropped != 3 || !strings.Contains(m.event.status, "cannot be replayed") || len(m.event.buffer.Records) != 1 {
		t.Fatal("terminal update lost last drop count, history, or replay warning")
	}
	m.Close()
}

func TestEventsFilterSearchPauseAndStaleGeneration(t *testing.T) {
	m := NewWithEvents(fakeGateway{}, nil, nil, fakeEventGateway{make(chan events.Update)}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 40, Height: 9})
	_ = m.openEvents()
	old := m.eventGeneration
	m = update(m, eventNext{context: "master-a", request: old, ok: true,
		update: events.Update{Record: events.Record{Tag: "salt/job/1/new", Data: json.RawMessage(`{"fun":"state.apply"}`)}}})
	m = update(m, eventNext{context: "master-a", request: old, ok: true,
		update: events.Update{Record: events.Record{Tag: "salt/key", Data: json.RawMessage(`{"id":"other"}`)}}})
	m = press(m, 'g')
	if len(m.eventIndices()) != 1 {
		t.Fatal("jobs category should filter buffered records")
	}
	m = press(m, '/')
	m = typeText(m, "state.apply")
	if len(m.eventIndices()) != 1 || !m.event.search.Focused() {
		t.Fatal("payload search did not retain matching event")
	}
	m = press(m, tea.KeyEscape)
	m = typeText(m, "p") // category p, not search text.
	if len(m.eventIndices()) != 0 || m.event.search.Value() != "state.apply" {
		t.Fatal("category change changed the query or included unrelated records")
	}
	m = press(m, 'r')
	if m.eventGeneration == old || len(m.event.buffer.Records) != 2 || !strings.Contains(m.event.status, "cannot be replayed") {
		t.Fatal("restart must keep buffered events and warn about gaps")
	}
	m = update(m, eventNext{context: "master-a", request: old, ok: true,
		update: events.Update{Record: events.Record{Tag: "stale"}}})
	if len(m.event.buffer.Records) != 2 {
		t.Fatal("previous subscription delivered into restarted view")
	}
	m.Close()
}

func TestEventsStartWhileJobsOpenAndSurviveViewSwitches(t *testing.T) {
	stream := make(chan events.Update, 2)
	m := NewWithEvents(fakeGateway{}, nil, nil, fakeEventGateway{stream}, "master-a", true)
	start := m.eventStartCmd()
	m = press(m, 'j')
	if m.activeView != 1 {
		t.Fatal("Jobs did not open")
	}
	next, wait := m.Update(start())
	m = next.(Model)
	if wait == nil {
		t.Fatal("late startup subscription was ignored outside Events")
	}
	stream <- events.Update{Record: events.Record{Tag: "salt/key", Data: json.RawMessage(`{"act":"accept"}`)}}
	next, wait = m.Update(wait())
	m = next.(Model)
	if len(m.event.buffer.Records) != 1 || wait == nil {
		t.Fatal("Events did not continue capturing while Jobs was open")
	}
	m = press(m, 'e')
	m = press(m, 'k')
	m = update(m, eventNext{context: m.context, request: m.eventGeneration, ok: true,
		update: events.Update{Record: events.Record{Tag: "salt/minion/node/start", Data: json.RawMessage(`{}`)}}})
	if len(m.event.buffer.Records) != 2 || m.activeView != 6 {
		t.Fatal("switching to Keys interrupted the event buffer")
	}
	m = press(m, 'f')
	m = press(m, 'e')
	if len(m.event.buffer.Records) != 2 || m.event.cancel == nil {
		t.Fatal("reopening Events lost history or listener")
	}
	m.Close()
}

func TestEventRestartPreservesLossAndShutdownCancels(t *testing.T) {
	m := NewWithEvents(fakeGateway{}, nil, nil, fakeEventGateway{make(chan events.Update)}, "master-a", true)
	initialModel := m // The program entry point also keeps this value for cleanup.
	initialCtx := m.event.ctx
	m = update(m, eventNext{context: m.context, request: m.eventGeneration, ok: true,
		update: events.Update{Record: events.Record{Tag: "salt/key", Data: json.RawMessage(`{}`)}, Dropped: 3}})
	old := m.eventGeneration
	_ = m.restartEvents()
	select {
	case <-initialCtx.Done():
	default:
		t.Fatal("restart did not cancel the previous listener")
	}
	m = update(m, eventNext{context: m.context, request: old, ok: true,
		update: events.Update{Record: events.Record{Tag: "stale"}, Dropped: 100}})
	m = update(m, eventNext{context: m.context, request: m.eventGeneration, ok: true,
		update: events.Update{Record: events.Record{Tag: "salt/job/1/new", Data: json.RawMessage(`{}`)}, Dropped: 2}})
	if m.event.streamDropped != 5 || len(m.event.buffer.Records) != 2 {
		t.Fatalf("lost records or cumulative drops across restart: %+v", m.event)
	}
	currentCtx := m.event.ctx
	initialModel.Close() // Covers Run terminating before it returns the last model.
	select {
	case <-currentCtx.Done():
	default:
		t.Fatal("shutdown from initial model did not cancel the restarted listener")
	}
	m.Close()
	m = update(m, eventNext{context: m.context, request: m.eventGeneration, ok: true,
		update: events.Update{Record: events.Record{Tag: "after shutdown", Data: json.RawMessage(`{}`)}}})
	if len(m.event.buffer.Records) != 2 {
		t.Fatal("event arrived after shutdown")
	}
}

func TestEventNewestFirstAndPausedSelection(t *testing.T) {
	m := NewWithEvents(fakeGateway{}, nil, nil, fakeEventGateway{make(chan events.Update)}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 8})
	_ = m.openEvents()
	// Receipt order, not an event-supplied timestamp, determines the newest row.
	for _, record := range []events.Record{
		{Tag: "salt/job/1/new", Stamp: "2099-01-01", Data: json.RawMessage(`{}`)},
		{Tag: "salt/job/2/new", Stamp: "2000-01-01", Data: json.RawMessage(`{}`)},
		{Tag: "salt/job/3/new", Data: json.RawMessage(`{}`)},
	} {
		m = update(m, eventNext{context: m.context, request: m.eventGeneration, ok: true, update: events.Update{Record: record}})
	}
	v := m.eventsViewData()
	if len(v.Rows) != 3 || v.Rows[0].Tag != "salt/job/3/new" || v.Rows[2].Tag != "salt/job/1/new" || v.Selected != 0 || v.Offset != 0 {
		t.Fatalf("auto-follow did not keep newest at top: %+v", v)
	}
	m = press(m, tea.KeySpace)
	m = press(m, tea.KeyEnter) // Inspect newest without losing it on arrival.
	m = update(m, eventNext{context: m.context, request: m.eventGeneration, ok: true,
		update: events.Update{Record: events.Record{Tag: "salt/job/4/new", Data: json.RawMessage(`{}`)}}})
	v = m.eventsViewData()
	if v.Rows[0].Tag != "salt/job/4/new" || v.Selected != 1 || v.Inspected != 1 || v.Offset != 1 || v.Unseen != 1 {
		t.Fatalf("paused inspection shifted under new event: %+v", v)
	}
	m = press(m, 'g') // Filter preserves the newest-first order.
	if got := m.eventsViewData().Rows; got[0].Tag != "salt/job/4/new" || got[1].Tag != "salt/job/3/new" {
		t.Fatalf("filtered events were reordered: %+v", got)
	}
	m = press(m, tea.KeySpace)
	v = m.eventsViewData()
	if v.Selected != 0 || v.Offset != 0 || v.Unseen != 0 || v.Rows[0].Tag != "salt/job/4/new" {
		t.Fatalf("resume did not follow newest row: %+v", v)
	}
	m.Close()
}

func TestPausedOldestEventEvictionSelectsNewest(t *testing.T) {
	m := NewWithEvents(fakeGateway{}, nil, nil, fakeEventGateway{make(chan events.Update)}, "master-a", true)
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 8})
	for i := 0; i < events.MaxCount; i++ {
		m.event.buffer.Add(events.Record{Tag: "old", Data: json.RawMessage(`{}`)})
	}
	m.event.paused, m.event.selected, m.event.inspect = true, 0, 0
	m = update(m, eventNext{context: m.context, request: m.eventGeneration, ok: true,
		update: events.Update{Record: events.Record{Tag: "newest", Data: json.RawMessage(`{}`)}}})
	v := m.eventsViewData()
	if len(v.Rows) != events.MaxCount || v.Rows[0].Tag != "newest" || v.Selected != 0 || v.Inspected != -1 || v.Offset != 0 || m.event.buffer.Dropped != 1 {
		t.Fatalf("eviction left a dangling selection or incorrect order: selected=%d inspected=%d offset=%d drops=%d", v.Selected, v.Inspected, v.Offset, m.event.buffer.Dropped)
	}
	m.Close()
}
