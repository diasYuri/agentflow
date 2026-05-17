package theme

import "testing"

func TestDefaultDarkTheme(t *testing.T) {
	tm := Default(ModeDark)
	if tm.Mode != ModeDark {
		t.Fatalf("expected dark mode, got %s", tm.Mode)
	}
	// Title should be bold in dark theme.
	if !tm.Title.GetBold() {
		t.Fatal("expected title style to be bold")
	}
}

func TestDefaultLightTheme(t *testing.T) {
	tm := Default(ModeLight)
	if tm.Mode != ModeLight {
		t.Fatalf("expected light mode, got %s", tm.Mode)
	}
}

func TestAutoResolvesToKnownMode(t *testing.T) {
	tm := Default(ModeAuto)
	if tm.Mode != ModeDark && tm.Mode != ModeLight {
		t.Fatalf("expected auto to resolve to dark or light, got %s", tm.Mode)
	}
}
