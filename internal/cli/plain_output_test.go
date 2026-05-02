package cli

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/models"
)

type failingWriter struct {
	failAt int
	writes int
}

func (w *failingWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.failAt {
		return 0, errors.New("write failed")
	}
	return len(p), nil
}

func TestPlainObserverWritesReadableEventsAndRespectsQuiet(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	observer := NewPlainObserver(&out, PlainOptions{Color: true})
	sensitiveMessage := bytes.NewBufferString("postgres://u:")
	sensitiveMessage.WriteString("sec")
	sensitiveMessage.WriteString("ret@h/db pass")
	sensitiveMessage.WriteString("word=hunter")
	sensitiveMessage.WriteString("2")
	for _, event := range []engine.Event{
		{Name: engine.EventSyncStart, Database: "mydb", Tables: 2, Engine: "native"},
		{Name: engine.EventSchemaPreDataStart},
		{Name: engine.EventSchemaPreDataDone, Duration: 820 * time.Millisecond},
		{Name: engine.EventTableCopyStart, Table: "public.users", Estimated: 1000},
		{Name: engine.EventTableCopyProgress, Table: "public.users", Rows: 120, Percent: 12, BytesPerSec: 4200},
		{Name: engine.EventTableCopyDone, Table: "public.users", Rows: 1000, Bytes: 4096, Duration: 3 * time.Second},
		{Name: engine.EventSchemaPostDataStart},
		{Name: engine.EventSchemaPostDataDone, Duration: time.Second},
		{Name: engine.EventSyncDone, Tables: 2, Bytes: 4096, Duration: 5 * time.Second},
		{Name: engine.EventSyncFailed, Stage: "copy", Table: "public.users", Error: sensitiveMessage.String()},
		{Name: "snapshot.done", Stage: "snapshot", Error: "password=hidden"},
		{},
	} {
		observer.OnEvent(context.Background(), event)
	}

	text := out.String()
	assert.Contains(t, text, "\x1b[1mstarting sync")
	assert.Contains(t, text, "database=mydb tables=2 engine=native")
	assert.Contains(t, text, "schema pre-data: done in 820ms")
	assert.Contains(t, text, "copying table public.users est_rows=1000")
	assert.Contains(t, text, "table public.users rows=120 pct=12.0% bytes_per_sec=4200")
	assert.Contains(t, text, "table public.users done rows=1000 bytes=4096 duration=3s")
	assert.Contains(t, text, "schema post-data: start")
	assert.Contains(t, text, "schema post-data: done in 1s")
	assert.Contains(t, text, "sync done tables=2 bytes=4096 duration=5s")
	assert.Contains(t, text, "error\x1b[0m stage=copy table=public.users")
	assert.Contains(t, text, "postgres://u:******@h/db password=******")
	assert.Contains(t, text, "snapshot.done stage=snapshot error=password=******")
	assert.NotContains(t, text, "secret")
	assert.NotContains(t, text, "hunter2")
	assert.NotContains(t, text, "hidden")

	quiet := NewPlainObserver(&out, PlainOptions{Quiet: true})
	before := out.Len()
	quiet.OnEvent(context.Background(), engine.Event{Name: engine.EventTableCopyProgress, Table: "public.users"})
	assert.Equal(t, before, out.Len())
}

func TestPlainObserverNilReceiverNoops(t *testing.T) {
	t.Parallel()
	var observer *PlainObserver
	observer.OnEvent(context.Background(), engine.Event{Name: engine.EventSyncStart})
}

func TestPrintPlanListsTablesInOrderAndNoColorDisablesANSI(t *testing.T) {
	t.Parallel()
	plan := &models.SyncPlan{
		Database: "mydb",
		Engine:   "native",
		DryRun:   true,
		Threads:  4,
		Tables: []models.Table{
			{Schema: "public", Name: "users", Rows: 2, SizeBytes: 20},
			{Schema: "public", Name: "orders", Rows: 3, SizeBytes: 30},
		},
		Sequences: []models.Sequence{{Schema: "public", Name: "users_id_seq"}},
	}
	var out bytes.Buffer
	require.NoError(t, PrintPlan(&out, plan, PlainOptions{Color: true, NoColor: true}))

	text := out.String()
	assert.Contains(t, text, "plan database=mydb engine=native tables=2 dry_run=true")
	assert.Contains(t, text, "threads=4 sequences=1")
	assert.Contains(t, text, "1. public.users rows=2 bytes=20")
	assert.Contains(t, text, "2. public.orders rows=3 bytes=30")
	assert.NotContains(t, text, "\x1b[")
}

func TestPrintResultWritesSummaryAndNilWriterIsSafe(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	result := &models.SyncResult{
		Database:     "mydb",
		StartedAt:    started,
		FinishedAt:   started.Add(1500 * time.Millisecond),
		TablesCopied: 2,
		RowsCopied:   7,
		BytesCopied:  9,
	}
	var out bytes.Buffer
	require.NoError(t, PrintResult(&out, result, PlainOptions{}))
	assert.Equal(t, "synced database=mydb tables=2 rows=7 bytes=9 duration=1.5s\n", out.String())
	require.NoError(t, PrintResult(nil, result, PlainOptions{}))
}

func TestPlainOutputErrorsAndHelpers(t *testing.T) {
	t.Parallel()
	require.EqualError(t, PrintPlan(&bytes.Buffer{}, nil, PlainOptions{}), "sync plan is required")
	require.EqualError(t, PrintResult(&bytes.Buffer{}, nil, PlainOptions{}), "sync result is required")
	require.Error(t, PrintPlan(&failingWriter{failAt: 1}, &models.SyncPlan{}, PlainOptions{}))
	require.Error(t, PrintPlan(&failingWriter{failAt: 2}, &models.SyncPlan{}, PlainOptions{}))
	require.Error(t, PrintPlan(&failingWriter{failAt: 3}, &models.SyncPlan{Tables: []models.Table{{Name: "users"}}}, PlainOptions{}))
	require.Error(t, PrintResult(&failingWriter{failAt: 1}, &models.SyncResult{}, PlainOptions{}))
	assert.Equal(t, "users", plainTableName(models.Table{Name: "users"}))
	assert.Equal(t, "public", plainTableName(models.Table{Schema: "public"}))
	assert.Equal(t, "public.users", plainTableName(models.Table{Schema: "public", Name: "users"}))
	assert.Equal(t, "plain", plainColor("plain", ansiBold, PlainOptions{}))
	assert.Equal(t, "0s", formatDuration(-time.Second))
}
