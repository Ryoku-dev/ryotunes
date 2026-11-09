// Package bus carries daemon events to whoever subscribed. Packages that
// produce events depend on Sink only; the socket server implements it and
// fans each event out to subscribed connections.
package bus

import "sync"

// Sink receives an event by name with a JSON-serialisable payload.
type Sink interface {
	Emit(event string, data any)
}

// SinkFunc adapts a function to Sink.
type SinkFunc func(event string, data any)

// Emit calls f.
func (f SinkFunc) Emit(event string, data any) { f(event, data) }

// Discard drops every event; use it in tests and before the server exists.
var Discard Sink = SinkFunc(func(string, any) {})

// Fanout forwards every event to all attached sinks. Attach and Detach are
// safe to call while events flow.
type Fanout struct {
	mu    sync.RWMutex
	sinks map[int]Sink
	next  int
}

// Attach adds s and returns a handle for Detach.
func (f *Fanout) Attach(s Sink) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sinks == nil {
		f.sinks = make(map[int]Sink)
	}
	f.next++
	f.sinks[f.next] = s
	return f.next
}

// Detach removes the sink registered under handle.
func (f *Fanout) Detach(handle int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sinks, handle)
}

// Len reports how many sinks are attached.
func (f *Fanout) Len() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.sinks)
}

// Emit delivers the event to every attached sink.
func (f *Fanout) Emit(event string, data any) {
	f.mu.RLock()
	sinks := make([]Sink, 0, len(f.sinks))
	for _, s := range f.sinks {
		sinks = append(sinks, s)
	}
	f.mu.RUnlock()
	for _, s := range sinks {
		s.Emit(event, data)
	}
}
