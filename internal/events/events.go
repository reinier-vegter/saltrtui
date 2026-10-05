// Package events defines normalized live master events and bounded retention.
package events

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	MaxFrame = 64 << 10
	MaxCount = 500
	MaxBytes = 2 << 20
)

type Record struct {
	Tag, Minion, JID, Stamp string
	Data                    json.RawMessage
	Observed                time.Time
}

type Update struct {
	Record  Record
	Dropped uint64 // Cumulative drops before this update.
	Err     error
}

type Gateway interface {
	Subscribe(ctx context.Context) (<-chan Update, error)
}

func ParseFrame(line []byte, at time.Time) (Record, error) {
	if len(line) == 0 || len(line) > MaxFrame {
		return Record{}, errors.New("event frame is empty or oversized")
	}
	parts := bytes.SplitN(bytes.TrimSuffix(line, []byte{'\r'}), []byte{'\t'}, 2)
	if len(parts) != 2 || len(parts[0]) == 0 || bytes.ContainsAny(parts[0], "\r\n") {
		return Record{}, errors.New("invalid event tag/frame")
	}
	dec := json.NewDecoder(bytes.NewReader(parts[1]))
	dec.UseNumber()
	var data map[string]json.RawMessage
	if err := dec.Decode(&data); err != nil || data == nil {
		return Record{}, errors.New("invalid event JSON object")
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return Record{}, errors.New("unexpected trailing event JSON")
	}
	text := func(key string) string {
		var value string
		_ = json.Unmarshal(data[key], &value)
		return value
	}
	tag := string(parts[0])
	record := Record{Tag: tag, Data: append(json.RawMessage(nil), parts[1]...), Observed: at,
		Minion: text("id"), JID: text("jid"), Stamp: text("_stamp")}
	fields := strings.Split(tag, "/")
	if len(fields) >= 3 && fields[0] == "salt" && fields[1] == "job" {
		if record.JID == "" {
			record.JID = fields[2]
		}
		if len(fields) >= 5 && record.Minion == "" && (fields[3] == "ret" || fields[3] == "start") {
			record.Minion = fields[4]
		}
	} else if len(fields) >= 4 && fields[0] == "salt" && fields[1] == "minion" && record.Minion == "" {
		record.Minion = fields[2]
	}
	return record, nil
}

type Buffer struct {
	Records []Record
	Bytes   int
	Dropped uint64
}

func (b *Buffer) Add(record Record) {
	b.Records = append(b.Records, record)
	b.Bytes += len(record.Tag) + len(record.Data)
	for len(b.Records) > MaxCount || b.Bytes > MaxBytes {
		old := b.Records[0]
		b.Bytes -= len(old.Tag) + len(old.Data)
		b.Records[0] = Record{}
		b.Records = b.Records[1:]
		b.Dropped++
	}
}

func (b *Buffer) Clear() { b.Records, b.Bytes = nil, 0 }

func Match(record Record, family, query string) bool {
	matchFamily := family == "all" || family == "" ||
		(family == "jobs" && strings.HasPrefix(record.Tag, "salt/job/")) ||
		(family == "minions" && strings.HasPrefix(record.Tag, "salt/minion/")) ||
		(family == "keys" && strings.HasPrefix(record.Tag, "salt/key")) ||
		(family == "presence" && strings.HasPrefix(record.Tag, "salt/presence/"))
	if !matchFamily {
		return false
	}
	return strings.Contains(strings.ToLower(fmt.Sprintf("%s %s %s %s", record.Tag, record.Minion, record.JID, record.Data)), strings.ToLower(query))
}
