package components

import (
	"testing"

	"github.com/diasYuri/agentflow/internal/tui/theme"
)

func TestNewKeyHelpDefaults(t *testing.T) {
	kh := NewKeyHelp()
	if len(kh.bindings) == 0 {
		t.Fatal("expected default bindings")
	}
}

func TestKeyHelpViewRenders(t *testing.T) {
	kh := NewKeyHelp()
	tm := theme.Default(theme.ModeDark)
	v := kh.View(tm, 80)
	if v == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestKeyHelpViewTruncates(t *testing.T) {
	kh := NewKeyHelp()
	tm := theme.Default(theme.ModeDark)
	v := kh.View(tm, 10)
	if v == "" {
		t.Fatal("expected non-empty truncated view")
	}
}

func TestKeyHelpViewZeroWidth(t *testing.T) {
	kh := NewKeyHelp()
	tm := theme.Default(theme.ModeDark)
	v := kh.View(tm, 0)
	if v != "" {
		t.Fatal("expected empty view for zero width")
	}
}

func TestSetBindings(t *testing.T) {
	kh := NewKeyHelp()
	kh.SetBindings([]Binding{{Key: "x", Description: "test"}})
	if len(kh.bindings) != 1 || kh.bindings[0].Key != "x" {
		t.Fatal("expected bindings to be updated")
	}
}

func TestTruncWidth(t *testing.T) {
	if truncWidth("hello", 10) != "hello" {
		t.Fatal("expected no truncation")
	}
	if truncWidth("hello world", 5) != "hell…" {
		t.Fatalf("expected truncation, got %s", truncWidth("hello world", 5))
	}
	if truncWidth("hello", 0) != "" {
		t.Fatal("expected empty for max 0")
	}
}
