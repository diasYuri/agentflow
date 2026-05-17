package animation

import (
	"testing"
	"time"
)

func TestNewConfig(t *testing.T) {
	c := NewConfig(true)
	if !c.ReducedMotion {
		t.Fatal("expected ReducedMotion to be true")
	}
	c2 := NewConfig(false)
	if c2.ReducedMotion {
		t.Fatal("expected ReducedMotion to be false")
	}
}

func TestTweenDurationWithReducedMotion(t *testing.T) {
	c := NewConfig(true)
	d := c.TweenDuration(500 * time.Millisecond)
	if d != 0 {
		t.Fatalf("expected 0 duration with reduced motion, got %v", d)
	}
}

func TestTweenDurationWithoutReducedMotion(t *testing.T) {
	c := NewConfig(false)
	d := c.TweenDuration(500 * time.Millisecond)
	if d != 500*time.Millisecond {
		t.Fatalf("expected 500ms, got %v", d)
	}
}

func TestInstant(t *testing.T) {
	c := NewConfig(true)
	if !c.Instant() {
		t.Fatal("expected Instant to be true with reduced motion")
	}
	c2 := NewConfig(false)
	if c2.Instant() {
		t.Fatal("expected Instant to be false without reduced motion")
	}
}

func TestSpringCreatesValue(t *testing.T) {
	c := NewConfig(false)
	s := c.Spring()
	pos, vel := s.Update(0, 0, 100)
	if pos == 0 && vel == 0 {
		t.Fatal("expected spring to produce non-zero movement")
	}
}

func TestProgressModelInstant(t *testing.T) {
	c := NewConfig(true)
	p := c.NewProgressModel()
	p.SetTarget(0.5)
	if p.Value() != 0.5 {
		t.Fatalf("expected instant value 0.5, got %f", p.Value())
	}
}

func TestProgressModelAnimates(t *testing.T) {
	c := NewConfig(false)
	p := c.NewProgressModel()
	p.SetTarget(1.0)
	if p.Value() == 1.0 {
		t.Fatal("expected animation not yet at target")
	}
	for i := 0; i < 100; i++ {
		p.Update()
	}
	if p.Value() < 0.9 {
		t.Fatalf("expected animation near target after 100 frames, got %f", p.Value())
	}
}

func TestProgressModelClamps(t *testing.T) {
	c := NewConfig(true)
	p := c.NewProgressModel()
	p.SetTarget(-0.5)
	if p.Value() != 0 {
		t.Fatalf("expected clamped to 0, got %f", p.Value())
	}
	p.SetTarget(1.5)
	if p.Value() != 1.0 {
		t.Fatalf("expected clamped to 1, got %f", p.Value())
	}
}
