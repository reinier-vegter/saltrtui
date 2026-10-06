package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"saltrtui/internal/metrics"
)

type GraphViewData struct {
	Width, Height               int
	ID, Context, Status, Source string
	Last                        time.Time
	Points                      []metrics.Point
	Busy, Paused, Help          bool
	Ready, Failed               bool
	CPUs                        int
	Duration, Interval          time.Duration
	Spacing                     time.Duration
}

var graphColors = []lipgloss.Style{
	lipgloss.NewStyle().Foreground(lipgloss.Color("81")),
	lipgloss.NewStyle().Foreground(lipgloss.Color("114")),
	lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
	lipgloss.NewStyle().Foreground(lipgloss.Color("177")),
}

func graphValue(p metrics.Point, metric, series int) (float64, bool) {
	switch metric {
	case 0:
		return p.CPU, p.CPUValid
	case 1:
		return p.Mem, p.MemValid
	case 2:
		return p.IO, p.CPUValid
	default:
		switch series {
		case 0:
			return p.Load.One, p.LoadValid
		case 1:
			return p.Load.Five, p.LoadValid
		default:
			return p.Load.Fifteen, p.LoadValid
		}
	}
}

func graphWindow(v GraphViewData) (time.Time, time.Duration) {
	end := v.Last
	if end.IsZero() && len(v.Points) > 0 {
		end = v.Points[len(v.Points)-1].At
	}
	span := time.Minute
	if len(v.Points) > 0 {
		span = max(span, end.Sub(v.Points[0].At))
	}
	return end, min(5*time.Minute, span)
}

// plotGraph rasterizes observed samples into Braille cells (2x4 dots/cell).
// Lines connect consecutive valid observations only, never failed samples or
// long sampling gaps. Grid cells stay visually subordinate to the series.
func plotGraph(v GraphViewData, metric, width, height int) []string {
	if width < 14 || height < 4 {
		return []string{muted.Render(clip("Enlarge terminal to plot", width))}
	}
	end, span := graphWindow(v)
	plotWidth, plotHeight := width-7, height-1
	cols, rows := plotWidth*2, plotHeight*4
	maximum := 100.0
	seriesCount := 1
	if metric == 3 {
		seriesCount, maximum = 3, max(1.0, float64(v.CPUs))
		for _, point := range v.Points {
			if point.LoadValid {
				maximum = max(maximum, point.Load.One, point.Load.Five, point.Load.Fifteen)
			}
		}
		maximum = math.Ceil(maximum)
	}
	type cell struct {
		dots  rune
		color int
	}
	cells := make([]cell, plotWidth*plotHeight)
	bits := [4][2]rune{{1, 8}, {2, 16}, {4, 32}, {64, 128}}
	put := func(x, y, color int) {
		x, y = max(0, min(cols-1, x)), max(0, min(rows-1, y))
		// Load histories remain distinguishable without ANSI color: solid 1m,
		// dashed 5m and dotted 15m (at sub-cell resolution).
		if metric == 3 && ((color == 3 && x%8 >= 4) || (color == 2 && x%6 != 0)) {
			return
		}
		c := &cells[(y/4)*plotWidth+x/2]
		c.dots |= bits[y%4][x%2]
		c.color = color
	}
	for series := 0; series < seriesCount; series++ {
		previousX, previousY, valid := 0, 0, false
		var previousAt time.Time
		for _, p := range v.Points {
			value, ok := graphValue(p, metric, series)
			if !ok || !metrics.Finite(value) || p.At.Before(end.Add(-span)) || p.At.After(end) {
				valid = false
				continue
			}
			x := int(float64(p.At.Sub(end.Add(-span))) / float64(span) * float64(cols-1))
			y := int(math.Round((1 - max(0, min(maximum, value))/maximum) * float64(rows-1)))
			color := metric
			if metric == 3 {
				color = []int{0, 3, 2}[series]
			}
			if valid && !p.BreakBefore && p.At.Sub(previousAt) <= max(6*time.Second, 2*v.Interval) {
				steps := max(abs(x-previousX), abs(y-previousY))
				for step := 0; step <= steps; step++ {
					t := float64(step) / float64(max(1, steps))
					put(previousX+int(math.Round(float64(x-previousX)*t)), previousY+int(math.Round(float64(y-previousY)*t)), color)
				}
			}
			put(x, y, color)
			previousX, previousY, previousAt, valid = x, y, p.At, true
		}
	}
	lines := make([]string, 0, height)
	for row := 0; row < plotHeight; row++ {
		label := "      "
		if row == 0 || row == plotHeight/2 || row == plotHeight-1 {
			value := maximum
			if row == plotHeight-1 {
				value = 0
			} else if row == plotHeight/2 {
				value = maximum / 2
			}
			label = fmt.Sprintf("%5.0f ", value)
			if metric == 3 {
				label = fmt.Sprintf("%5.1f ", value)
			}
		}
		var line strings.Builder
		line.WriteString(muted.Render(label + "│"))
		for col := 0; col < plotWidth; col++ {
			c := cells[row*plotWidth+col]
			if c.dots != 0 {
				line.WriteString(graphColors[c.color].Render(string(rune(0x2800) + c.dots)))
			} else if metric == 3 && v.CPUs > 0 && row == int(math.Round((1-float64(v.CPUs)/maximum)*float64(plotHeight-1))) {
				line.WriteString(muted.Render("┈"))
			} else if row == 0 || row == plotHeight/2 || row == plotHeight-1 {
				line.WriteString(muted.Render("┄"))
			} else {
				line.WriteByte(' ')
			}
		}
		lines = append(lines, line.String())
	}
	left, right := fmt.Sprintf("−%ds", int(span.Seconds())), "now"
	axis := left + strings.Repeat(" ", max(0, plotWidth-len([]rune(left))-len(right))) + right
	lines = append(lines, muted.Render("      └"+axis))
	return lines
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func graphHeading(v GraphViewData, metric int, width int) string {
	name := []string{"CPU busy", "Memory used", "I/O wait", "Load average"}[metric]
	if metric == 3 && width < 40 {
		name = "Load"
	}
	latest := "—"
	if len(v.Points) > 0 {
		p := v.Points[len(v.Points)-1]
		value, valid := graphValue(p, metric, 0)
		if valid {
			latest = fmt.Sprintf("%.1f%%", value)
			if metric == 1 {
				latest += fmt.Sprintf(" · %.1f/%.1f GiB", float64(p.Memory.Total-p.Memory.Available)/(1<<30), float64(p.Memory.Total)/(1<<30))
			}
			if metric == 3 {
				latest = fmt.Sprintf("1m %.2f · 5m %.2f · 15m %.2f", p.Load.One, p.Load.Five, p.Load.Fifteen)
			}
		}
	}
	style := graphColors[metric]
	return style.Bold(true).Render(clip(name+"  "+latest, width))
}

func graphBody(v GraphViewData, width, height int) []string {
	if v.Help {
		return styledLines([]string{"Resource graph keys", "esc: close and return focus to Fleet", "space: pause/resume · r: sample now · ?: help", "", "One compound Salt job per sample; target interval 2s.", "CPU and I/O wait need two consecutive counter reads.", "Memory excludes available/cache-reclaimable memory.", "Load is runnable/waiting work, not a percentage.", "Load: solid cyan 1m · dashed purple 5m · dotted amber 15m.", "Gaps mean unavailable data, not zero utilization.", "History: last 5 minutes, up to 300 points, memory only."}, width)
	}
	if width < 28 || height < 8 {
		return styledLines([]string{"Terminal too small for resource plots.", "Enlarge terminal · esc closes"}, width)
	}
	grid := width >= 56 && height >= 10
	plotWidth, plotHeight := width, height/4-1
	if grid {
		plotWidth, plotHeight = (width-3)/2, height/2-1
	}
	var charts [4][]string
	for metric := range charts {
		charts[metric] = append([]string{graphHeading(v, metric, plotWidth)}, plotGraph(v, metric, plotWidth, plotHeight)...)
	}
	var lines []string
	if grid {
		for _, pair := range [][2]int{{0, 1}, {2, 3}} {
			for row := 0; row < len(charts[pair[0]]); row++ {
				lines = append(lines, fixed(charts[pair[0]][row], plotWidth)+"   "+fixed(charts[pair[1]][row], plotWidth))
			}
		}
	} else if plotHeight >= 4 {
		for _, chart := range charts {
			lines = append(lines, chart...)
		}
	} else {
		// Short terminals still show one real shared percentage plot and the
		// four readings instead of silently reverting to tiny sparklines.
		for metric := range charts {
			lines = append(lines, graphHeading(v, metric, width))
		}
		lines = append(lines, plotGraph(v, 0, width, height-4)...)
	}
	return lines
}

// RenderGraph renders only the pop-over, in its allocated cell rectangle.
func RenderGraph(v GraphViewData) string {
	w, h := max(0, v.Width), max(0, v.Height)
	if w == 0 || h == 0 {
		return ""
	}
	if w < 6 || h < 4 {
		lines := make([]string, h)
		for i := range lines {
			lines[i] = fixed(clip("esc closes · resize", w), w)
		}
		return strings.Join(lines, "\n")
	}
	inner := w - 4
	title := clip(" Resources · "+v.ID+" ", w-4)
	lines := []string{accent.Render("╭─" + title + strings.Repeat("─", max(0, w-3-lipgloss.Width(title))) + "╮")}
	mode := "live"
	if !v.Ready {
		mode = "unavailable"
	}
	if v.Paused {
		mode = "paused"
	} else if v.Busy {
		mode = "sampling"
		if !v.Ready {
			mode = "discovering"
		}
	}
	summary := fmt.Sprintf("%s · %s · target %s · request %s", mode, v.Source, v.Interval, v.Duration.Round(time.Millisecond))
	if v.Spacing > 0 {
		summary += " · actual " + v.Spacing.Round(time.Millisecond).String()
	}
	if !v.Last.IsZero() {
		summary += " · last " + v.Last.Local().Format("15:04:05")
	}
	if v.CPUs > 0 {
		summary += fmt.Sprintf(" · %d CPUs", v.CPUs)
	}
	body := []string{muted.Render(clip(summary, inner))}
	body = append(body, graphBody(v, inner, h-6)...)
	status := v.Status
	if !v.Paused && !v.Busy && status == "" {
		status = "Load: 1m ━ cyan · 5m ┄ purple · 15m · amber"
		if v.CPUs > 0 {
			status += fmt.Sprintf(" · ┈ CPU reference %d", v.CPUs)
		}
	}
	// Reserve the last two interior rows for status and the close route.
	for len(body) < h-4 {
		body = append(body, "")
	}
	if len(body) > h-4 {
		body = body[:h-4]
	}
	statusStyle := muted
	if v.Failed && !v.Busy {
		statusStyle = failure
	}
	body = append(body, statusStyle.Render(clip(status, inner)), accent.Render(clip("esc close · space pause/resume · r sample · ? help", inner)))
	for _, line := range body {
		lines = append(lines, accent.Render("│")+" "+fixed(line, inner)+" "+accent.Render("│"))
	}
	lines = append(lines, accent.Render("╰"+strings.Repeat("─", w-2)+"╯"))
	return lipgloss.NewStyle().Background(lipgloss.Color("235")).Render(strings.Join(lines, "\n"))
}

func GraphOverlaySize(width, height int) (int, int) {
	return max(0, width*4/5), max(0, height*4/5)
}

func RenderGraphOverlay(fleet string, v GraphViewData) string {
	terminalWidth, terminalHeight := v.Width, v.Height
	w, h := GraphOverlaySize(v.Width, v.Height)
	x, y := (v.Width-w)/2, (v.Height-h)/2
	if w == 0 || h == 0 {
		return fleet
	}
	v.Width, v.Height = w, h
	result := lipgloss.NewCompositor(lipgloss.NewLayer(fleet), lipgloss.NewLayer(RenderGraph(v)).X(x).Y(y).Z(1)).Render()
	lines := strings.Split(result, "\n")
	for len(lines) < terminalHeight {
		lines = append(lines, "")
	}
	// Composition may trim empty trailing cells; the root frame stays exactly
	// terminal-sized, including very small terminals and wide Unicode labels.
	for i := range lines {
		lines[i] = fixed(lines[i], terminalWidth)
	}
	return strings.Join(lines, "\n")
}
