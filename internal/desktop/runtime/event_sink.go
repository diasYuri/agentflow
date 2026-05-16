package runtime

import (
	"context"
	"sync"

	"github.com/diasYuri/agentflow/internal/core/ports"
	"github.com/diasYuri/agentflow/internal/core/run"
)

// FanOutSink multiplexa eventos para multiplos sinks de forma segura para concorrencia.
type FanOutSink struct {
	mu    sync.RWMutex
	sinks []ports.EventSink
}

// NewFanOutSink cria um novo sink de fan-out.
func NewFanOutSink(sinks ...ports.EventSink) *FanOutSink {
	return &FanOutSink{sinks: sinks}
}

// AddSink adiciona um sink dinamicamente.
func (f *FanOutSink) AddSink(sink ports.EventSink) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sinks = append(f.sinks, sink)
}

// RemoveSink remove um sink.
func (f *FanOutSink) RemoveSink(sink ports.EventSink) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, s := range f.sinks {
		if s == sink {
			f.sinks = append(f.sinks[:i], f.sinks[i+1:]...)
			break
		}
	}
}

// Emit envia o evento para todos os sinks registrados.
func (f *FanOutSink) Emit(ctx context.Context, event run.Event) error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, sink := range f.sinks {
		if err := sink.Emit(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// Close fecha todos os sinks registrados.
func (f *FanOutSink) Close(ctx context.Context) error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var first error
	for _, sink := range f.sinks {
		if err := sink.Close(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}
