// Package fleet defines Salt-independent Fleet observations.
package fleet

import (
	"context"
	"sort"
	"strings"
	"time"
)

// Grains is the minion-reported snapshot. Values can be scalar, arrays, or
// nested maps; a grain's source cannot be inferred from the returned value.
type Grains map[string]any

type Gateway interface {
	ListAcceptedKeys(context.Context) ([]string, error)
	ReadPresence(context.Context) ([]string, error)
	ReadSelectedGrains(context.Context, string) (Grains, error)
}

type Observation[T any] struct {
	Value T
	At    time.Time
	Err   error
	Busy  bool
}

// FilterIDs searches locally. The Salt gateway never receives search text.
func FilterIDs(ids []string, query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	var result []string
	for _, id := range ids {
		if strings.Contains(strings.ToLower(id), query) {
			result = append(result, id)
		}
	}
	return result
}

func SortUnique(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	sort.Strings(result)
	return result
}
