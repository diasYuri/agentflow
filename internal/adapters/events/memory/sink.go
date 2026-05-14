package memory

import (
	"context"
	"sync"

	"github.com/diasYuri/agentflow/internal/core/run"
)

type Sink struct {
	mu     sync.Mutex
	Events []run.Event
}

func New() *Sink { return &Sink{} }

func (s *Sink) Emit(ctx context.Context, event run.Event) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Events = append(s.Events, event)
	return nil
}

func (s *Sink) Close(ctx context.Context) error {
	_ = ctx
	return nil
}
