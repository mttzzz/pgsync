package engine_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/engine"
)

func TestObserverFuncForwardsEvent(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), observerContextKey{}, "marker")
	event := engine.Event{Name: engine.EventSyncStart, Database: "app"}
	var gotCtx context.Context
	var gotEvent engine.Event

	observer := engine.ObserverFunc(func(ctx context.Context, event engine.Event) {
		gotCtx = ctx
		gotEvent = event
	})
	observer.OnEvent(ctx, event)

	assert.Same(t, ctx, gotCtx)
	assert.Equal(t, event, gotEvent)
}

func TestObserverFuncNilIsNoop(t *testing.T) {
	t.Parallel()
	var observer engine.ObserverFunc
	require.NotPanics(t, func() {
		observer.OnEvent(context.Background(), engine.Event{Name: engine.EventSyncStart})
	})
}

func TestMultiObserverNilIsNoop(t *testing.T) {
	t.Parallel()
	var observer engine.MultiObserver
	require.NotPanics(t, func() {
		observer.OnEvent(context.Background(), engine.Event{Name: engine.EventSyncStart})
	})
}

func TestMultiObserverSkipsNilObservers(t *testing.T) {
	t.Parallel()
	calls := 0
	observer := engine.MultiObserver{
		nil,
		engine.ObserverFunc(func(context.Context, engine.Event) {
			calls++
		}),
		nil,
	}

	require.NotPanics(t, func() {
		observer.OnEvent(context.Background(), engine.Event{Name: engine.EventSyncStart})
	})
	assert.Equal(t, 1, calls)
}

func TestMultiObserverPreservesOrdering(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	event := engine.Event{Name: engine.EventTableCopyProgress, Table: "public.users"}
	calls := make([]string, 0, 3)
	observer := engine.MultiObserver{
		engine.ObserverFunc(func(gotCtx context.Context, gotEvent engine.Event) {
			assert.Equal(t, ctx, gotCtx)
			assert.Equal(t, event, gotEvent)
			calls = append(calls, "first")
		}),
		engine.ObserverFunc(func(gotCtx context.Context, gotEvent engine.Event) {
			assert.Equal(t, ctx, gotCtx)
			assert.Equal(t, event, gotEvent)
			calls = append(calls, "second")
		}),
		engine.ObserverFunc(func(gotCtx context.Context, gotEvent engine.Event) {
			assert.Equal(t, ctx, gotCtx)
			assert.Equal(t, event, gotEvent)
			calls = append(calls, "third")
		}),
	}

	observer.OnEvent(ctx, event)
	assert.Equal(t, []string{"first", "second", "third"}, calls)
}

type observerContextKey struct{}
