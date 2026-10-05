// Package console defines Salt command results and in-memory history behavior.
package console

import (
	"context"
	"strconv"
	"strings"
)

type Result struct {
	Stdout, Stderr string
	Retcode        int
}

type Identity struct {
	Account, Home, Cwd string
}

type Gateway interface {
	Identity(context.Context, string) (Identity, error)
	History(context.Context, string, string) ([]string, error)
	Run(context.Context, string, string, string) (Result, error)
	ChangeDirectory(context.Context, string, string) (string, error)
}

const MaxHistoryEntries = 500

// ParseHistory reads a bounded tail of Bash's saved history. With timestamps,
// the timestamp is an entry separator and subsequent lines form one command.
func ParseHistory(raw string) []string {
	raw = strings.TrimSuffix(raw, "\n")
	if raw == "" {
		return nil
	}
	var entries []string
	var current []string
	marked := false
	flush := func() {
		if len(current) > 0 {
			entries = append(entries, strings.Join(current, "\n"))
			current = nil
		}
	}
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "#") {
			if _, err := strconv.ParseInt(line[1:], 10, 64); err == nil {
				flush()
				marked = true
				continue
			}
		}
		if marked {
			current = append(current, line)
		} else if line != "" {
			entries = append(entries, line)
		}
	}
	flush()
	if len(entries) > MaxHistoryEntries {
		entries = entries[len(entries)-MaxHistoryEntries:]
	}
	return entries
}

type HistoryState struct {
	Saved, Session []string
	index          int
	draft          string
}

func (h *HistoryState) ResetNavigation() { h.index = 0; h.draft = "" }

func (h *HistoryState) entries() []string {
	return append(append([]string(nil), h.Saved...), h.Session...)
}

func (h *HistoryState) Up(value string) string {
	entries := h.entries()
	if len(entries) == 0 {
		return value
	}
	if h.index == 0 {
		h.draft = value
		h.index = len(entries) + 1
	}
	if h.index > 1 {
		h.index--
	} else {
		h.index = 1
	}
	return entries[h.index-1]
}

func (h *HistoryState) Down(value string) string {
	entries := h.entries()
	if h.index == 0 || len(entries) == 0 {
		return value
	}
	if h.index < len(entries) {
		h.index++
		return entries[h.index-1]
	}
	h.index = 0
	return h.draft
}

func (h *HistoryState) Add(command string) {
	if command != "" {
		h.Session = append(h.Session, command)
		if len(h.Session) > MaxHistoryEntries {
			h.Session = h.Session[len(h.Session)-MaxHistoryEntries:]
		}
	}
	h.ResetNavigation()
}

// Reverse returns the newest matching entry before before, or a sentinel -1.
func (h *HistoryState) Reverse(query string, before int) (int, string) {
	entries := h.entries()
	if before > len(entries) {
		before = len(entries)
	}
	for i := before - 1; i >= 0; i-- {
		if strings.Contains(strings.ToLower(entries[i]), strings.ToLower(query)) {
			return i, entries[i]
		}
	}
	return -1, ""
}

func (h *HistoryState) Len() int { return len(h.Saved) + len(h.Session) }
