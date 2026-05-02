//go:build integration

// Package integration contains Docker-backed integration tests for pgsync.
package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/test/helpers"
)

func TestNativeSyncCancellationDuringCopyFailsAndRetrySucceeds(t *testing.T) {
	skipIfSystemPgDumpUnavailable(t)
	baseCtx, cancelBase := integrationContext(t)
	defer cancelBase()

	source, target := startSyncContainers(baseCtx, t)
	loadTinyFixture(baseCtx, t, source)
	loadCancellationRows(baseCtx, t, source)

	eng := newNativeEngine(t)
	plan, err := eng.Plan(baseCtx, syncPlanOptions(source, target, nil, false))
	if err != nil {
		t.Fatalf("plan cancellable native sync: %v", err)
	}

	copyCtx, cancelCopy := context.WithCancel(baseCtx)
	recorder := &recordingObserver{cancel: cancelCopy, cancelOn: engine.EventTableCopyStart}
	result, err := eng.Execute(copyCtx, plan, recorder)
	cancelCopy()
	if err == nil {
		t.Fatalf("expected canceled sync to return an error, result=%+v", result)
	}
	if !isCancellationError(err) {
		t.Fatalf("expected cancellation error, got %v", err)
	}
	assertCopyFailureEvent(t, recorder.Events())

	retryRecorder := &recordingObserver{}
	retryResult, err := eng.Execute(baseCtx, plan, retryRecorder)
	if err != nil {
		t.Fatalf("retry native sync after cancellation: %v", err)
	}
	assertSuccessfulResult(t, retryResult, len(tinySyncTables))
	assertEventsContainInOrder(t, eventNames(retryRecorder.Events()), []string{
		engine.EventSyncStart,
		engine.EventTableCopyDone,
		engine.EventSyncDone,
	})
}

func loadCancellationRows(ctx context.Context, t testing.TB, source helpers.PostgresContainer) {
	t.Helper()
	execSQL(ctx, t, source, `
INSERT INTO public.users (id, email, name, active, created_at)
SELECT 1000 + gs, 'bulk-' || gs || '@example.test', 'Bulk User ' || gs, true, '2026-04-01 00:00:00+00'::timestamptz
FROM generate_series(1, 1500) AS gs;

INSERT INTO public.orders (id, user_id, status, total_cents, placed_at)
SELECT 2000 + gs, 1000 + gs, 'paid', gs, '2026-04-02 00:00:00+00'::timestamptz
FROM generate_series(1, 1500) AS gs;

INSERT INTO public.order_items (id, order_id, sku, quantity, unit_price_cents)
SELECT 3000 + gs, 2000 + gs, 'bulk-sku-' || gs, 1, gs
FROM generate_series(1, 1500) AS gs;

SELECT setval(pg_get_serial_sequence('public.users', 'id'), (SELECT max(id) FROM public.users), true);
SELECT setval(pg_get_serial_sequence('public.orders', 'id'), (SELECT max(id) FROM public.orders), true);
SELECT setval(pg_get_serial_sequence('public.order_items', 'id'), (SELECT max(id) FROM public.order_items), true);`)
}

func isCancellationError(err error) bool {
	return errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "context canceled")
}

func assertCopyFailureEvent(t testing.TB, events []engine.Event) {
	t.Helper()
	for _, event := range events {
		if event.Stage == "copy" && strings.HasSuffix(event.Name, ".failed") {
			return
		}
	}
	t.Fatalf("expected copy failure event, got %v", eventNames(events))
}
