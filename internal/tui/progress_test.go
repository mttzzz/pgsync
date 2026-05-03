package tui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/models"
)

func TestLiveProgressAggregatesCopyEvents(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	plan := &models.SyncPlan{Tables: []models.Table{{Schema: "public", Name: "users", Rows: 10, SizeBytes: 100}}}
	progress := NewLiveProgress(plan, started)

	assert.Equal(t, 1, progress.TablesTotal)
	assert.Equal(t, int64(10), progress.RowsEstimated)
	assert.Equal(t, int64(100), progress.BytesEstimated)
	assert.Empty(t, NewLiveProgress(nil, started).planTables)

	progress.Apply(engine.Event{Name: engine.EventSyncStart, Tables: 1, Time: started}, started)
	progress.Apply(engine.Event{Name: engine.EventTableCopyStart, Table: "public.users", Time: started.Add(time.Second), Estimated: 100}, started.Add(time.Second))
	progress.Apply(engine.Event{Name: engine.EventTableCopyProgress, Table: "public.users", Bytes: 40, Percent: 40, BytesPerSec: 20}, started.Add(2*time.Second))
	assert.Equal(t, int64(40), progress.BytesCopied)
	assert.Equal(t, 40.0, progress.TablePercent)
	assert.Equal(t, 40.0, progress.OverallPercent)
	assert.Equal(t, float64(20), progress.BytesPerSec)

	progress.Apply(engine.Event{Name: engine.EventTableCopyDone, Table: "public.users", Rows: 10, Bytes: 120, Duration: 3 * time.Second}, started.Add(4*time.Second))
	assert.Equal(t, 1, progress.TablesDone)
	assert.Equal(t, int64(10), progress.RowsCopied)
	assert.Equal(t, int64(120), progress.BytesCopied)
	assert.Equal(t, 100.0, progress.TablePercent)
	assert.Equal(t, 100.0, progress.OverallPercent)
	assert.Len(t, progress.TableResults, 1)

	snapshot := progress.Snapshot(config.Config{Runtime: config.Runtime{DefaultDatabase: "app"}}, 120)
	assert.Equal(t, "app", snapshot.Header.Database)
	assert.Equal(t, int64(120), snapshot.BytesCopied)
	assert.NotEmpty(t, snapshot.Events)

	progress.Apply(engine.Event{Name: engine.EventSyncFailed}, started.Add(5*time.Second))
	assert.Equal(t, 1, progress.Errors)
	progress.Tick(started.Add(6 * time.Second))
	assert.Equal(t, started.Add(6*time.Second), progress.Now)
}

func TestLiveProgressCoversZeroStateAndListCaps(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	var progress LiveProgress
	progress.Apply(engine.Event{Name: engine.EventSyncStart, Tables: 3}, now)
	assert.Equal(t, now, progress.StartedAt)
	assert.Equal(t, 3, progress.TablesTotal)
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
		progress.recordTableResult(engine.Event{Table: "public.users", Rows: 1, Bytes: 1, Duration: time.Second})
	}
	assert.Len(t, progress.TableResults, 10)
}

func TestLiveProgressFallbacksAndEventDetails(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	progress := NewLiveProgress(&models.SyncPlan{}, now)
	progress.Apply(engine.Event{Name: engine.EventTableCopyStart, Table: "ad_hoc", Estimated: 50}, now)
	assert.Equal(t, int64(50), progress.CurrentBytesEstimate)

	progress.BytesEstimated = 0
	progress.TablesTotal = 2
	progress.TablesDone = 1
	progress.TablePercent = 50
	progress.recalculateOverall()
	assert.Equal(t, 75.0, progress.OverallPercent)

	progress.TablesTotal = 0
	progress.TablePercent = -10
	progress.recalculateOverall()
	assert.Equal(t, 0.0, progress.OverallPercent)
	progress.TablePercent = 110
	progress.recalculateOverall()
	assert.Equal(t, 100.0, progress.OverallPercent)

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
