package events

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestParseFrameAndFiltering(t *testing.T) {
	at := time.Now()
	record, err := ParseFrame([]byte("salt/job/123/ret/web-1\t{\"return\":{\"large\":9007199254740993},\"_stamp\":\"2026-09-28\"}\r"), at)
	if err != nil || record.JID != "123" || record.Minion != "web-1" || record.Stamp != "2026-09-28" || !record.Observed.Equal(at) {
		t.Fatalf("normalized return frame: %+v, %v", record, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(record.Data, &payload); err != nil {
		t.Fatal(err)
	}
	if !Match(record, "jobs", "9007199254740993") || Match(record, "keys", "web-1") || !Match(record, "all", "WEB-1") {
		t.Fatal("category or text filter lost the event")
	}
	for _, frame := range []string{"", "salt/job/123/new {}", "tag\t[]", "tag\t{} {}", "tag\tbroken", "tag\t", "\t{}"} {
		if _, err := ParseFrame([]byte(frame), at); err == nil {
			t.Fatalf("accepted malformed event frame %q", frame)
		}
	}
	if _, err := ParseFrame([]byte(strings.Repeat("x", MaxFrame+1)), at); err == nil {
		t.Fatal("accepted oversized event frame")
	}
}

func TestBufferBoundAndClearRetainsGapCount(t *testing.T) {
	var b Buffer
	for i := 0; i <= MaxCount; i++ {
		b.Add(Record{Tag: "salt/key", Data: json.RawMessage(`{}`)})
	}
	if len(b.Records) != MaxCount || b.Dropped != 1 || b.Bytes != MaxCount*(len("salt/key")+2) {
		t.Fatalf("count retention: %+v", b)
	}
	b.Clear()
	if len(b.Records) != 0 || b.Bytes != 0 || b.Dropped != 1 {
		t.Fatalf("local clear hid recorded loss: %+v", b)
	}
	b.Add(Record{Tag: "large", Data: json.RawMessage(strings.Repeat("x", MaxBytes))})
	if len(b.Records) != 0 || b.Dropped != 2 || b.Bytes != 0 {
		t.Fatalf("oversized event should be dropped: %+v", b)
	}
}
