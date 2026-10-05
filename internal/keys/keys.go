// Package keys describes master-side Salt public keys without CLI types.
package keys

import (
	"context"
	"sort"
	"strings"
	"time"
)

type State string

const (
	Pending  State = "pending"
	Accepted State = "accepted"
	Rejected State = "rejected"
	Denied   State = "denied"
)

type Key struct {
	ID    string
	State State
}

type Detail struct {
	Fingerprint string
	// FileTime is an approximate pending-key file modification time, not an
	// authoritative timestamp for the minion's first announcement.
	FileTime time.Time
}

type Action string

const (
	Accept Action = "accept"
	Block  Action = "block"
	Revoke Action = "revoke"
)

func (a Action) Expected() State {
	if a == Revoke {
		return Accepted
	}
	return Pending
}

type Gateway interface {
	ListKeys(context.Context) ([]Key, error)
	ReadKey(context.Context, Key) (Detail, error)
	ChangeKey(context.Context, Key, Action) error
}

func Filter(items []Key, query string) []Key {
	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]Key, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.ID+" "+string(item.State)), query) {
			result = append(result, item)
		}
	}
	return result
}

func Sort(items []Key) {
	order := map[State]int{Pending: 0, Accepted: 1, Rejected: 2, Denied: 3}
	sort.Slice(items, func(i, j int) bool {
		if items[i].State != items[j].State {
			return order[items[i].State] < order[items[j].State]
		}
		return items[i].ID < items[j].ID
	})
}
