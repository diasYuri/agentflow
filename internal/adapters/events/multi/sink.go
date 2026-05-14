package multi

import (
	"context"

	"github.com/diasYuri/agentflow/internal/core/ports"
	"github.com/diasYuri/agentflow/internal/core/run"
)

type Sink struct {
	sinks []ports.EventSink
}

func New(sinks ...ports.EventSink) *Sink {
	return &Sink{sinks: sinks}
}

func (s *Sink) Emit(ctx context.Context, event run.Event) error {
	for _, sink := range s.sinks {
		if err := sink.Emit(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (s *Sink) Close(ctx context.Context) error {
	var first error
	for _, sink := range s.sinks {
		if err := sink.Close(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (s *Sink) Open(path string) error {
	for _, sink := range s.sinks {
		if opener, ok := sink.(interface{ Open(string) error }); ok {
			if err := opener.Open(path); err != nil {
				return err
			}
		}
	}
	return nil
}
