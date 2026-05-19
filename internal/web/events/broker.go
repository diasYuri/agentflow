// Package events implements an in-process pub/sub broker used to
// fan out chat events (messages, tool-call updates, diagnostics) to
// any number of SSE subscribers.
//
// The broker only owns the live publish path. Replay of past events
// is handled by the persistence layer; callers compose the two when
// they want to support reconnects.
package events

import (
	"sync"
	"sync/atomic"
	"time"
)

// Kind tags an event payload so the frontend can route updates.
type Kind string

const (
	KindMessage     Kind = "message"
	KindToolCall    Kind = "tool_call"
	KindApproval    Kind = "approval"
	KindDiagnostic  Kind = "diagnostic"
	KindError       Kind = "error"
	KindHeartbeat   Kind = "heartbeat"
	KindSessionInfo Kind = "session"
)

// Event is the contract carried over SSE. ID is monotonically
// increasing per session so reconnecting clients can request a
// replay from the persistence layer using the same value.
type Event struct {
	ID            uint64    `json:"id"`
	SessionID     string    `json:"session_id,omitempty"`
	Kind          Kind      `json:"kind"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
	Payload       any       `json:"payload,omitempty"`
}

// Broker fans Events out to subscribers. The zero value is not safe
// for use; create instances via NewBroker.
type Broker struct {
	mu        sync.RWMutex
	channels  map[string]map[*subscriber]struct{}
	nextID    uint64
	closed    bool
	bufferLen int
}

// Subscription is the handle returned by Subscribe.
type Subscription struct {
	C       <-chan Event
	cancel  func()
	dropped *uint64
}

// Close releases the subscription. It is safe to call multiple times.
func (s *Subscription) Close() {
	if s.cancel != nil {
		s.cancel()
	}
}

// Dropped returns the number of events that were skipped because the
// receiver was too slow.
func (s *Subscription) Dropped() uint64 {
	if s.dropped == nil {
		return 0
	}
	return atomic.LoadUint64(s.dropped)
}

type subscriber struct {
	ch      chan Event
	dropped uint64
}

// NewBroker creates a broker with a per-subscriber buffer of
// bufferLen events. A small buffer keeps memory low while still
// tolerating short consumer hiccups.
func NewBroker(bufferLen int) *Broker {
	if bufferLen <= 0 {
		bufferLen = 32
	}
	return &Broker{
		channels:  make(map[string]map[*subscriber]struct{}),
		bufferLen: bufferLen,
	}
}

// Publish records an event for sessionID and pushes a copy to every
// active subscriber. Subscribers that cannot keep up have older
// events dropped instead of blocking the broker.
func (b *Broker) Publish(sessionID string, kind Kind, payload any, correlationID string) Event {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return Event{}
	}
	b.nextID++
	event := Event{
		ID:            b.nextID,
		SessionID:     sessionID,
		Kind:          kind,
		CorrelationID: correlationID,
		OccurredAt:    time.Now().UTC(),
		Payload:       payload,
	}
	subs := b.snapshotLocked(sessionID)
	b.mu.Unlock()
	for sub := range subs {
		select {
		case sub.ch <- event:
		default:
			atomic.AddUint64(&sub.dropped, 1)
		}
	}
	return event
}

// Subscribe returns a Subscription that receives every Event published
// for sessionID. When sessionID is empty the subscriber receives
// every event broadcast by the broker.
func (b *Broker) Subscribe(sessionID string) *Subscription {
	sub := &subscriber{ch: make(chan Event, b.bufferLen)}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.channels[sessionID] == nil {
		b.channels[sessionID] = make(map[*subscriber]struct{})
	}
	b.channels[sessionID][sub] = struct{}{}
	return &Subscription{
		C:       sub.ch,
		dropped: &sub.dropped,
		cancel: func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if subs := b.channels[sessionID]; subs != nil {
				if _, ok := subs[sub]; ok {
					delete(subs, sub)
					close(sub.ch)
					if len(subs) == 0 {
						delete(b.channels, sessionID)
					}
				}
			}
		},
	}
}

// Close releases every subscriber and prevents further publishes.
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for sessionID, subs := range b.channels {
		for sub := range subs {
			close(sub.ch)
		}
		delete(b.channels, sessionID)
	}
}

// NextID returns the most recent event ID the broker has issued. It
// is exposed so unit tests can confirm ordering invariants.
func (b *Broker) NextID() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.nextID
}

func (b *Broker) snapshotLocked(sessionID string) map[*subscriber]struct{} {
	out := make(map[*subscriber]struct{})
	for sub := range b.channels[sessionID] {
		out[sub] = struct{}{}
	}
	for sub := range b.channels[""] {
		out[sub] = struct{}{}
	}
	return out
}
