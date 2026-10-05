package ui

import (
	"fmt"
	"strings"
	"time"

	"saltrtui/internal/metrics"
)

type GraphViewData struct {
	Width, Height       int
	ID, Context, Status string
	Last                time.Time
	Points              []metrics.Point
	Busy, Paused, Help  bool
}

func graphSeries(points []metrics.Point, width int, metric string) (string, string) {
	blocks := []rune("▁▂▃▄▅▆▇█")
	if width <= 0 {
		return "", "—"
	}
	if len(points) > width {
		points = points[len(points)-width:]
	}
	var series strings.Builder
	current := "—"
	for _, point := range points {
		valid, value := false, 0.0
		switch metric {
		case "cpu":
			valid, value = point.CPUValid, point.CPU
		case "iowait":
			valid, value = point.CPUValid, point.IO
		case "memory":
			valid, value = point.MemValid, point.Mem
		}
		if !valid {
			series.WriteRune('·')
			current = "—"
			continue
		}
		value = max(0, min(100, value))
		series.WriteRune(blocks[min(7, int(value*8/100))])
		current = fmt.Sprintf("%.1f%%", value)
	}
	return series.String(), current
}

func graphLines(v GraphViewData, width int) []string {
	lines := []string{"Linux resource samples · 0–100% · most recent at right", ""}
	if v.Last.IsZero() {
		lines = append(lines, "Waiting for first sample…", "")
	} else {
		lines = append(lines, "Last sample: "+v.Last.Local().Format("15:04:05"), "")
	}
	for _, row := range []struct{ label, metric string }{{"CPU busy", "cpu"}, {"I/O wait", "iowait"}, {"Memory used", "memory"}} {
		graph, latest := graphSeries(v.Points, max(0, width), row.metric)
		lines = append(lines, fmt.Sprintf("%s: %s", row.label, latest), graph, "")
	}
	lines = append(lines, "· = no valid sample; CPU needs two consecutive counter reads.",
		"10s after each completed poll · 120 samples max · in-memory only")
	return styledLines(lines, width)
}

func compactGraphLines(v GraphViewData, width int) []string {
	lines := []string{}
	for _, row := range []struct{ label, metric string }{{"CPU", "cpu"}, {"I/O", "iowait"}, {"Mem", "memory"}} {
		graph, latest := graphSeries(v.Points, max(0, width-13), row.metric)
		lines = append(lines, item.Render(clip(fmt.Sprintf("%-3s %5s %s", row.label, latest, graph), width)))
	}
	return lines
}

func RenderGraph(v GraphViewData) string {
	w, h := max(0, v.Width), max(0, v.Height)
	if w == 0 || h == 0 {
		return ""
	}
	hints := []hint{{"esc", "Fleet"}, {"space", "pause/resume"}, {"r", "sample now"}, {"?", "help"}, {"q", "quit"}}
	notice := ""
	alert := false
	if v.Paused {
		hints, notice = []hint{{"space", "resume"}, {"esc", "Fleet"}, {"?", "help"}, {"q", "quit"}}, "Paused"
	}
	if v.Status != "" && !v.Busy && !v.Paused {
		hints, notice, alert = []hint{{"r", "retry"}, {"esc", "Fleet"}, {"?", "help"}}, v.Status, true
	}
	if h == 1 {
		return actionBar(w, notice, alert, hints...)
	}
	frame := []string{modeBar(w, "Fleet", v.Context, "Resources · "+v.ID)}
	if h >= 4 {
		frame = append(frame, muted.Render(strings.Repeat("─", w)))
	}
	bodyHeight := h - len(frame) - 1
	if bodyHeight > 0 {
		var lines []string
		title := "CPU · I/O wait · memory"
		if v.Help {
			title = "Resource graph keys"
			lines = styledLines([]string{"Fleet g: watch selected minion", "esc: return to Fleet",
				"space: pause/resume polling", "r: sample immediately when idle", "?: toggle help", "q: quit", "",
				"Each read is a Salt job. CPU needs two valid counter reads.",
				"Missing or reset counters make gaps, not interpolated data."}, w-4)
		} else {
			lines = graphLines(v, w-4)
			if bodyHeight < 17 {
				lines = compactGraphLines(v, w-4)
			}
			if v.Busy {
				title += " · sampling"
			}
			if v.Paused {
				title += " · paused"
			}
			if v.Status != "" {
				lines = append(lines, styledLines([]string{"", "Status: " + v.Status}, w-4)...)
			}
		}
		frame = append(frame, strings.Split(panel(title, lines, 0, w, bodyHeight, true, nil), "\n")...)
	}
	frame = append(frame, actionBar(w, notice, alert, hints...))
	return strings.Join(frame, "\n")
}
