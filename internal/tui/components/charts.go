package components

import (
	"fmt"
	"strings"

	"github.com/NimbleMarkets/ntcharts/barchart"
	"github.com/diasYuri/agentflow/internal/tui/theme"
)

// BarChart renders a bar chart using ntcharts with a textual fallback when dimensions are too small.
func BarChart(t *theme.Theme, width, height int, labels []string, values []float64, title string) string {
	if width < 20 || height < 6 || len(labels) == 0 || len(values) == 0 {
		return barChartFallback(t, width, labels, values, title)
	}
	data := make([]barchart.BarData, 0, len(labels))
	maxVal := 0.0
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal <= 0 {
		maxVal = 1
	}
	for i, label := range labels {
		if i >= len(values) {
			break
		}
		v := values[i]
		style := t.Primary
		if v == maxVal {
			style = t.Success
		}
		data = append(data, barchart.BarData{
			Label: label,
			Values: []barchart.BarValue{
				{Name: "", Value: v, Style: style},
			},
		})
	}
	bc := barchart.New(width, height, barchart.WithDataSet(data), barchart.WithNoAutoBarWidth())
	bc.Draw()
	return bc.View()
}

// StatusBarChart renders a horizontal bar chart of status counts.
func StatusBarChart(t *theme.Theme, width, height int, statuses map[string]int) string {
	if width < 20 || height < 6 || len(statuses) == 0 {
		return statusBarChartFallback(t, width, statuses)
	}
	labels := make([]string, 0, len(statuses))
	values := make([]float64, 0, len(statuses))
	for s, c := range statuses {
		labels = append(labels, s)
		values = append(values, float64(c))
	}
	return BarChart(t, width, height, labels, values, "")
}

func barChartFallback(t *theme.Theme, width int, labels []string, values []float64, title string) string {
	var b strings.Builder
	if title != "" {
		b.WriteString(t.Subtitle.Render(trunc(title, width)) + "\n")
	}
	maxVal := 0.0
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal <= 0 {
		maxVal = 1
	}
	barWidth := width - 12
	if barWidth < 5 {
		barWidth = 5
	}
	for i, label := range labels {
		if i >= len(values) {
			break
		}
		v := values[i]
		filled := int(v * float64(barWidth) / maxVal)
		if filled < 0 {
			filled = 0
		}
		if filled > barWidth {
			filled = barWidth
		}
		bar := t.Primary.Render(strings.Repeat("█", filled)) + t.Muted.Render(strings.Repeat("░", barWidth-filled))
		b.WriteString(fmt.Sprintf("%s %s %v\n", trunc(label, 8), bar, v))
	}
	return b.String()
}

func statusBarChartFallback(t *theme.Theme, width int, statuses map[string]int) string {
	var b strings.Builder
	b.WriteString(t.Subtitle.Render("Status Counts") + "\n")
	maxVal := 0
	for _, c := range statuses {
		if c > maxVal {
			maxVal = c
		}
	}
	if maxVal <= 0 {
		maxVal = 1
	}
	barWidth := width - 14
	if barWidth < 5 {
		barWidth = 5
	}
	for status, count := range statuses {
		style := t.Primary
		switch status {
		case "success", "completed":
			style = t.Success
		case "failed", "error":
			style = t.Danger
		case "cancelled", "canceled":
			style = t.Muted
		case "paused":
			style = t.Warning
		}
		filled := count * barWidth / maxVal
		bar := style.Render(strings.Repeat("█", filled)) + t.Muted.Render(strings.Repeat("░", barWidth-filled))
		b.WriteString(fmt.Sprintf("%s %s %d\n", trunc(status, 10), bar, count))
	}
	return b.String()
}
