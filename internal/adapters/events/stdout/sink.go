package stdout

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/diasYuri/agentflow/internal/core/run"
)

type Sink struct {
	w      io.Writer
	format string
}

func New(w io.Writer, format string) *Sink {
	return &Sink{w: w, format: format}
}

func (s *Sink) Emit(ctx context.Context, event run.Event) error {
	_ = ctx
	if s.format == "json" {
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(s.w, string(data))
		return err
	}
	if warning, ok := event.Data["warning"].(string); ok && warning != "" {
		if event.NodeID != "" {
			_, err := fmt.Fprintf(s.w, "[%s] %s %s: %s\n", event.Timestamp.Format(time.RFC3339), event.NodeID, event.Type, warning)
			return err
		}
		_, err := fmt.Fprintf(s.w, "[%s] %s: %s\n", event.Timestamp.Format(time.RFC3339), event.Type, warning)
		return err
	}
	if event.NodeID != "" {
		_, err := fmt.Fprintf(s.w, "[%s] %s %s\n", event.Timestamp.Format(time.RFC3339), event.NodeID, event.Type)
		return err
	}
	_, err := fmt.Fprintf(s.w, "[%s] %s\n", event.Timestamp.Format(time.RFC3339), event.Type)
	return err
}

func (s *Sink) Close(ctx context.Context) error {
	_ = ctx
	return nil
}
