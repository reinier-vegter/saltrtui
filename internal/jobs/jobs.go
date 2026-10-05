// Package jobs defines cache-backed, read-only Salt job observations.
package jobs

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
)

type Summary struct {
	JID, Function, Target, TargetType, StartTime string
}

type Return struct {
	Value   json.RawMessage
	Retcode *int
	Success *bool
}

type Detail struct {
	Summary
	Minions []string
	Returns map[string]Return
}

type Gateway interface {
	ListRecentJobs(context.Context) ([]Summary, error)
	ReadJob(context.Context, string) (Detail, error)
}

func Filter(items []Summary, query string) []Summary {
	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]Summary, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.JID+" "+item.Function+" "+item.Target), query) {
			result = append(result, item)
		}
	}
	return result
}

func Sort(items []Summary) {
	sort.Slice(items, func(i, j int) bool { return items[i].JID > items[j].JID })
}
