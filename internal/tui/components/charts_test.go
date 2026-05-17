package components

import (
	"strings"
	"testing"

	"github.com/diasYuri/agentflow/internal/tui/theme"
)

func TestBarChartFallbackWhenTooSmall(t *testing.T) {
	labels := []string{"a", "b"}
	values := []float64{10, 20}
	out := BarChart(theme.Default(theme.ModeDark), 5, 3, labels, values, "title")
	if out == "" {
		t.Fatal("expected fallback output")
	}
}

func TestBarChartRendersBars(t *testing.T) {
	labels := []string{"a", "b", "c"}
	values := []float64{10, 20, 5}
	out := BarChart(theme.Default(theme.ModeDark), 40, 10, labels, values, "title")
	if out == "" {
		t.Fatal("expected chart output")
	}
}

func TestStatusBarChartFallback(t *testing.T) {
	statuses := map[string]int{"running": 2, "success": 5}
	out := StatusBarChart(theme.Default(theme.ModeDark), 5, 3, statuses)
	if out == "" {
		t.Fatal("expected fallback output")
	}
	if !strings.Contains(out, "running") {
		t.Fatal("expected running status in fallback")
	}
}

func TestStatusBarChartRenders(t *testing.T) {
	statuses := map[string]int{"running": 2, "success": 5, "failed": 1}
	out := StatusBarChart(theme.Default(theme.ModeDark), 40, 10, statuses)
	if out == "" {
		t.Fatal("expected chart output")
	}
}

func TestBarChartEmptyData(t *testing.T) {
	out := BarChart(theme.Default(theme.ModeDark), 40, 10, nil, nil, "empty")
	if out == "" {
		t.Fatal("expected fallback output for empty data")
	}
}
