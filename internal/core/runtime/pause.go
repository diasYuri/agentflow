package runtime

import "sync"

// PauseController lets callers request a graceful pause for an active run.
// The runtime checks Requested at checkpoint-safe boundaries and finalizes the
// current run as paused while keeping the checkpoint file for resume.
type PauseController struct {
	mu        sync.Mutex
	requested bool
}

// NewPauseController returns a PauseController in the initial (not requested) state.
func NewPauseController() *PauseController {
	return &PauseController{}
}

// Request marks a pause as requested. The next checkpoint boundary in the
// runtime will honor it.
func (c *PauseController) Request() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.requested = true
	c.mu.Unlock()
}

// Requested reports whether a pause has been requested.
func (c *PauseController) Requested() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requested
}

// Reset clears the requested flag. Useful when reusing the controller across
// runs (e.g. after resume succeeds).
func (c *PauseController) Reset() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.requested = false
	c.mu.Unlock()
}
