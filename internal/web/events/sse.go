package events

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Stream writes Events from sub to w in the SSE wire format, until
// the request context is cancelled or the subscription closes. A
// periodic heartbeat keeps proxies from closing the connection.
func Stream(ctx context.Context, w http.ResponseWriter, sub *Subscription, heartbeat time.Duration) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("events: response writer does not support streaming")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
		return err
	}
	flusher.Flush()
	if heartbeat <= 0 {
		heartbeat = 15 * time.Second
	}
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-sub.C:
			if !ok {
				return nil
			}
			if err := writeEvent(w, ev); err != nil {
				return err
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return err
			}
			flusher.Flush()
		}
	}
}

func writeEvent(w http.ResponseWriter, ev Event) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	if _, err := fmt.Fprintf(w, "id: %d\n", ev.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", ev.Kind); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return err
	}
	return nil
}

// WriteEvent writes a single SSE frame for ev. The caller is
// responsible for flushing the writer.
func WriteEvent(w http.ResponseWriter, ev Event) error {
	return writeEvent(w, ev)
}
