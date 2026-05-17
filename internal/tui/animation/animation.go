// Package animation wraps Harmonica for future TUI animations.
package animation

import (
	"math"
	"time"

	"github.com/charmbracelet/harmonica"
)

// Config holds animation preferences.
type Config struct {
	ReducedMotion bool
}

// NewConfig creates an animation config.
func NewConfig(reducedMotion bool) Config {
	return Config{ReducedMotion: reducedMotion}
}

// Spring creates a new spring animation.
func (c Config) Spring() harmonica.Spring {
	return harmonica.NewSpring(harmonica.FPS(60), 1.0, 0.2)
}

// TweenDuration returns an effective duration, collapsing to 0 when reduced motion is enabled.
func (c Config) TweenDuration(d time.Duration) time.Duration {
	if c.ReducedMotion {
		return 0
	}
	return d
}

// Instant returns true when animations should be skipped.
func (c Config) Instant() bool {
	return c.ReducedMotion
}

// ProgressSpring creates a spring for progress bar animations.
func (c Config) ProgressSpring() harmonica.Spring {
	if c.ReducedMotion {
		return harmonica.NewSpring(harmonica.FPS(60), 0, 0)
	}
	return harmonica.NewSpring(harmonica.FPS(60), 1.2, 0.25)
}

// ProgressModel holds animated progress state.
type ProgressModel struct {
	spring   harmonica.Spring
	target   float64
	current  float64
	velocity float64
	instant  bool
}

// NewProgressModel creates an animated progress model.
func (c Config) NewProgressModel() ProgressModel {
	return ProgressModel{
		spring:  c.ProgressSpring(),
		instant: c.Instant(),
	}
}

// SetTarget updates the target progress value (0-1).
func (p *ProgressModel) SetTarget(v float64) {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	p.target = v
	if p.instant {
		p.current = v
	}
}

// Update advances the animation by one frame.
func (p *ProgressModel) Update() {
	if p.instant || math.Abs(p.target-p.current) < 0.001 {
		p.current = p.target
		return
	}
	p.current, p.velocity = p.spring.Update(p.current, p.velocity, p.target)
}

// Value returns the current animated progress.
func (p *ProgressModel) Value() float64 {
	return p.current
}
