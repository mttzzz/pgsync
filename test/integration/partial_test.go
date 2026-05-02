//go:build integration

// Package integration contains Docker-backed integration tests for pgsync.
package integration

import (
	"context"
	"testing"

	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/test/helpers"
)

var partialClosureTables = []string{
	"public.users",
	"public.orders",
	"public.order_items",
}

func TestPartialPlanIncludesFKClosureAndExcludesUnrelatedTables(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	source, target := startSyncContainers(ctx, t)
	loadTinyAndPartialFixtures(ctx, t, source)

	plan := planPartialSync(ctx, t, source, target)
	assertPlanIncludesOnlyClosure(t, plan.Tables)
}

func TestPartialSyncCopiesOnlyFKClosureData(t *testing.T) {
	skipIfSystemPgDumpUnavailable(t)
	ctx, cancel := integrationContext(t)
	defer cancel()

	source, target := startSyncContainers(ctx, t)
	loadTinyAndPartialFixtures(ctx, t, source)

	plan := planPartialSync(ctx, t, source, target)
	recorder := &recordingObserver{}
	result, err := newNativeEngine(t).Execute(ctx, plan, recorder)
	if err != nil {
		t.Fatalf("execute partial native sync: %v", err)
	}
	assertSuccessfulResult(t, result, len(partialClosureTables))
	assertEventsContainInOrder(t, eventNames(recorder.Events()), []string{
		engine.EventSyncStart,
		engine.EventTableCopyStart,
		engine.EventTableCopyDone,
		engine.EventSyncDone,
	})

	helpers.AssertTableRowCountsEqual(ctx, t, source, target, partialClosureTables)
	helpers.AssertTableChecksumsEqual(ctx, t, source, target, partialClosureTables)
	helpers.AssertSequencesUsable(ctx, t, target, partialClosureTables)
	assertUnrelatedAuditLogHasNoCopiedRows(ctx, t, target)
}

func loadTinyAndPartialFixtures(ctx context.Context, t testing.TB, source helpers.PostgresContainer) {
	t.Helper()
	loadTinyFixture(ctx, t, source)
	loadPartialFixture(ctx, t, source)
}

func planPartialSync(
	ctx context.Context,
	t testing.TB,
	source helpers.PostgresContainer,
	target helpers.PostgresContainer,
) *models.SyncPlan {
	t.Helper()
	plan, err := newNativeEngine(t).Plan(ctx, syncPlanOptions(source, target, []string{"order_items"}, false))
	if err != nil {
		t.Fatalf("plan partial native sync: %v", err)
	}
	return plan
}

func assertPlanIncludesOnlyClosure(t testing.TB, tables []models.Table) {
	t.Helper()
	for _, table := range partialClosureTables {
		if !containsTable(tables, table) {
			t.Fatalf("expected partial plan to include %s, got %s", table, formatTables(tables))
		}
	}
	if containsTable(tables, "public.audit_log") {
		t.Fatalf("expected partial plan to exclude public.audit_log, got %s", formatTables(tables))
	}
	if len(tables) != len(partialClosureTables) {
		t.Fatalf("expected only FK closure tables %v, got %s", partialClosureTables, formatTables(tables))
	}
}

func assertUnrelatedAuditLogHasNoCopiedRows(ctx context.Context, t testing.TB, target helpers.PostgresContainer) {
	t.Helper()
	count, exists := auditLogRowCountIfExists(ctx, t, target)
	if !exists {
		return
	}
	// Phase 2 may apply the full pre-data schema, but partial data copy must stay filtered to the FK closure.
	if count != 0 {
		t.Fatalf("expected unrelated public.audit_log to have zero copied rows, got %d", count)
	}
}
