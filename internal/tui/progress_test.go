package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/models"
)

func TestLiveProgressSingleDBPlanFillsEstimates(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	plan := &models.SyncPlan{Database: "app", Tables: []models.Table{{Schema: "public", Name: "users", Rows: 10, SizeBytes: 100}}}
	progress := NewLiveProgress(plan, now)

	assert.Equal(t, 1, progress.QueueTablesTotal)
	assert.Equal(t, 1, progress.DBTablesTotal)
	assert.Equal(t, int64(10), progress.QueueRowsEstimated)
	assert.Equal(t, int64(10), progress.DBRowsEstimated)
	assert.Equal(t, int64(100), progress.QueueBytesEstimated)
	assert.Equal(t, int64(100), progress.DBBytesEstimated)
	assert.Equal(t, "app", progress.CurrentDatabase)
	assert.Empty(t, NewLiveProgress(nil, now).planTables)
}

func TestLiveProgressApplyAggregatesAcrossTableLifecycle(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	plan := &models.SyncPlan{Database: "app", Tables: []models.Table{{Schema: "public", Name: "users", Rows: 10, SizeBytes: 100}}}
	progress := NewLiveProgress(plan, now)

	progress.Apply(engine.Event{Name: engine.EventSyncStart, Database: "app", Tables: 1, Time: now}, now)
	progress.Apply(engine.Event{Name: engine.EventTableCopyStart, Database: "app", Table: "public.users", Time: now.Add(time.Second), Estimated: 100}, now.Add(time.Second))
	progress.Apply(engine.Event{Name: engine.EventTableCopyProgress, Database: "app", Table: "public.users", Bytes: 40, Percent: 40, BytesPerSec: 20}, now.Add(2*time.Second))
	assert.Equal(t, int64(40), progress.QueueBytesCopied())
	assert.Equal(t, int64(40), progress.DBBytesCopied())
	assert.Equal(t, 40.0, progress.CurrentPercent)
	assert.Equal(t, 0.0, progress.QueuePercent(), "row-based queue % is 0 until table done")
	assert.Equal(t, float64(20), progress.CurrentBytesPerSec)

	progress.Apply(engine.Event{Name: engine.EventTableCopyDone, Database: "app", Table: "public.users", Rows: 10, Bytes: 120, Duration: 3 * time.Second}, now.Add(4*time.Second))
	assert.Equal(t, 1, progress.QueueTablesDone)
	assert.Equal(t, 1, progress.DBTablesDone)
	assert.Equal(t, int64(10), progress.QueueRowsCopied())
	assert.Equal(t, int64(120), progress.QueueBytesCopied())
	assert.Equal(t, "", progress.CurrentTable, "current table cleared after done")
	assert.Equal(t, int64(0), progress.CurrentBytes)
	assert.Len(t, progress.TableResults, 1)
	assert.InDelta(t, 100.0, progress.QueuePercent(), 0.5)

	snapshot := progress.Snapshot(config.Config{Runtime: config.Runtime{DefaultDatabase: "fallback"}}, 120)
	assert.Equal(t, "app", snapshot.Header.Database)
	assert.Equal(t, int64(120), snapshot.QueueBytesCopied)
	assert.NotEmpty(t, snapshot.Events)

	progress.Apply(engine.Event{Name: engine.EventSyncFailed}, now.Add(5*time.Second))
	assert.Equal(t, 1, progress.Errors)
	progress.Tick(now.Add(6 * time.Second))
	assert.Equal(t, now.Add(6*time.Second), progress.Now)
}

func TestLiveProgressZeroStateAndListCaps(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	var progress LiveProgress
	progress.Apply(engine.Event{Name: engine.EventSyncStart, Database: "alpha"}, now)
	assert.Equal(t, now, progress.StartedAt)
	assert.Equal(t, "alpha", progress.CurrentDatabase)
	assert.Equal(t, 1, progress.DBIndex)

	progress.Tick(now.Add(time.Second))
	assert.Equal(t, now.Add(time.Second), progress.Now)
	var tickingOnly LiveProgress
	tickingOnly.Tick(now)
	assert.Equal(t, now, tickingOnly.StartedAt)

	for i := 0; i < 129; i++ {
		progress.prependEvent(engine.Event{Name: engine.EventTableCopyProgress, Time: now})
	}
	assert.Len(t, progress.Events, 128)
	for i := 0; i < 11; i++ {
		progress.recordTableResult(engine.Event{Database: "alpha", Table: "public.users", Rows: 1, Bytes: 1, Duration: time.Second})
	}
	assert.Len(t, progress.TableResults, 11, "all per-table results retained for the final report")
	assert.Equal(t, "alpha", progress.TableResults[0].Database)
}

func TestLiveProgressFallbacksAndEventDetails(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	progress := NewLiveProgress(&models.SyncPlan{}, now)
	progress.Apply(engine.Event{Name: engine.EventTableCopyStart, Table: "ad_hoc", Estimated: 50}, now)
	assert.Equal(t, int64(50), progress.CurrentBytesEstimate)

	assert.Equal(t, 0.0, safePercent(0, 0), "zero denominator yields 0%")
	assert.Equal(t, 100.0, safePercent(150, 100), "overflow clamps to 100%")
	assert.Equal(t, 0.0, safePercent(-1, 100), "negative clamps to 0%")

	assert.Equal(t, 5.0, easeToward(0, 20), "moves a quarter toward target")
	assert.Equal(t, 10.0, easeToward(50, 10), "snaps down on reduced target")

	assert.Equal(t, "custom", eventStageLabel(engine.Event{Stage: "custom", Name: "name"}))
	assert.Equal(t, "name", eventStageLabel(engine.Event{Name: "name"}))
	assert.Equal(t, "waiting", eventStageLabel(engine.Event{}))
	assert.Equal(t, "disk est 1.0 KB", eventDetails(engine.Event{Name: engine.EventTableCopyStart, Estimated: 1024}))
	assert.Contains(t, eventDetails(engine.Event{Name: engine.EventTableCopyProgress, Bytes: 1024, Percent: 50, BytesPerSec: 1024}), "COPY stream")
	assert.Contains(t, eventDetails(engine.Event{Name: engine.EventTableCopyDone, Rows: 7, Bytes: 2048, Duration: time.Second}), "7 rows")
	assert.Equal(t, "1.0s", eventDetails(engine.Event{Name: engine.EventSchemaPreDataDone, Duration: time.Second}))
	assert.Equal(t, "boom", eventDetails(engine.Event{Name: engine.EventSyncFailed, Error: "boom"}))
	assert.Equal(t, "err", eventDetails(engine.Event{Error: "err"}))
	assert.Equal(t, "1.0s", eventDetails(engine.Event{Duration: time.Second}))
	assert.Empty(t, eventDetails(engine.Event{}))

	assert.Equal(t, "public.users", tableEventName(models.Table{Schema: "public", Name: "users"}))
	assert.Equal(t, "users", tableEventName(models.Table{Name: "users"}))
	assert.Equal(t, "public", tableEventName(models.Table{Schema: "public"}))
	assert.Equal(t, "fallback", emptyValue(" ", "fallback"))
	assert.Equal(t, "value", emptyValue("value", "fallback"))
	assert.Equal(t, 2, maxInt(2, 1))
	assert.Equal(t, 2, maxInt(1, 2))
}

func TestLiveProgressForQueueRegistersDBPlans(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	queue := []models.Database{
		{Name: "alpha", SizeBytes: 1024, TableCount: 2},
		{Name: "beta", SizeBytes: 2048, TableCount: 3},
	}
	progress := NewLiveProgressForQueue(queue, now)
	assert.Equal(t, 2, progress.DBTotal)
	assert.Equal(t, 5, progress.QueueTablesTotal)
	assert.Equal(t, int64(3072), progress.QueueBytesEstimated)
	assert.Equal(t, int64(0), progress.QueueRowsEstimated, "rows unknown until DB plans land")

	progress.RegisterDB("alpha", []models.Table{
		{Schema: "public", Name: "users", Rows: 100, SizeBytes: 500},
		{Schema: "public", Name: "orders", Rows: 200, SizeBytes: 600},
	}, 300, 1100)
	assert.Equal(t, 1, progress.DBIndex)
	assert.Equal(t, "alpha", progress.CurrentDatabase)
	assert.Equal(t, int64(1100), progress.DBBytesEstimated)
	assert.Equal(t, int64(300), progress.DBRowsEstimated)
	assert.Equal(t, 2, progress.DBTablesTotal)
	assert.Equal(t, int64(300), progress.QueueRowsEstimated)
	assert.Contains(t, progress.planTables, "public.users")

	progress.Apply(engine.Event{Name: engine.EventTableCopyStart, Database: "alpha", Table: "public.users"}, now.Add(time.Second))
	assert.Equal(t, int64(100), progress.CurrentRowsEstimate, "row estimate comes from registered plan")
	assert.Equal(t, int64(500), progress.CurrentBytesEstimate)

	progress.Apply(engine.Event{Name: engine.EventTableCopyDone, Database: "alpha", Table: "public.users", Rows: 100, Bytes: 500, Duration: time.Second}, now.Add(2*time.Second))
	assert.Equal(t, 1, progress.DBTablesDone)
	assert.Equal(t, 1, progress.QueueTablesDone)
	assert.Equal(t, int64(500), progress.DBBytesCopied())
	assert.Equal(t, int64(500), progress.QueueBytesCopied())

	progress.RegisterDB("beta", []models.Table{{Schema: "public", Name: "events", Rows: 50, SizeBytes: 800}}, 50, 800)
	assert.Equal(t, 2, progress.DBIndex)
	assert.Equal(t, int64(0), progress.DBBytesCopied(), "DB counters reset on new registration")
	assert.Equal(t, int64(500), progress.QueueBytesCopied(), "queue counters survive across DBs")
	assert.Equal(t, int64(350), progress.QueueRowsEstimated)

	progress.RegisterDB("alpha", nil, 0, 0)
	assert.Equal(t, 2, progress.DBIndex, "revisiting alpha must not bump DBIndex")
}

func TestLiveProgressForQueueGrowsDBTotalWhenEventsExceedQueue(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	progress := NewLiveProgressForQueue([]models.Database{{Name: "alpha"}}, now)
	progress.Apply(engine.Event{Name: engine.EventSyncStart, Database: "alpha"}, now)
	progress.Apply(engine.Event{Name: engine.EventSyncStart, Database: "beta"}, now.Add(time.Second))
	assert.Equal(t, 2, progress.DBIndex)
	assert.Equal(t, 2, progress.DBTotal, "DBTotal grows when more DBs arrive than expected")
}

func TestRegisterDBGrowsDBTotalBeyondInitialQueue(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	progress := NewLiveProgressForQueue([]models.Database{{Name: "alpha"}}, now)
	progress.RegisterDB("alpha", nil, 0, 0)
	progress.RegisterDB("beta", nil, 0, 0)
	assert.Equal(t, 2, progress.DBTotal)
}

func TestRegisterDBLazilyInitializesSeenMap(t *testing.T) {
	t.Parallel()
	var progress LiveProgress
	progress.RegisterDB("alpha", []models.Table{{Schema: "public", Name: "users", Rows: 1, SizeBytes: 1}}, 1, 1)
	assert.Equal(t, 1, progress.DBIndex)
	assert.Equal(t, "alpha", progress.CurrentDatabase)
	assert.Contains(t, progress.planTables, "public.users")
}

func TestRecordTableResultFallsBackToCurrentDatabase(t *testing.T) {
	t.Parallel()
	progress := LiveProgress{CurrentDatabase: "fallback"}
	progress.recordTableResult(engine.Event{Table: "public.x", Rows: 1, Bytes: 1, Duration: 0})
	assert.Equal(t, "fallback", progress.TableResults[0].Database)
	assert.Equal(t, 0.0, progress.TableResults[0].Speed, "speed stays zero when duration is zero")
}
