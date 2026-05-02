package engine

import "context"

// ProgressObserver receives engine progress events.
type ProgressObserver interface {
	OnEvent(ctx context.Context, event Event)
}

// ObserverFunc adapts a function into a ProgressObserver.
type ObserverFunc func(ctx context.Context, event Event)

// OnEvent calls f with the event, or no-ops when f is nil.
func (f ObserverFunc) OnEvent(ctx context.Context, event Event) {
	if f == nil {
		return
	}
	f(ctx, event)
}

// MultiObserver fan-outs events to multiple observers in order.
type MultiObserver []ProgressObserver

// OnEvent sends the event to every non-nil observer in order.
func (m MultiObserver) OnEvent(ctx context.Context, event Event) {
	for _, observer := range m {
		if observer == nil {
			continue
		}
		observer.OnEvent(ctx, event)
	}
}
