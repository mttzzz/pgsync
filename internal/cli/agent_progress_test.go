package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/engine"
)

type cliStaticClock struct {
	now time.Time
}

func (c cliStaticClock) Now() time.Time { return c.now }

func TestNDJSONObserverEmitsSpecEvents(t *testing.T) {
	t.Parallel()
	fixed := time.Date(2026, 5, 2, 10, 11, 23, 0, time.UTC)
	var out bytes.Buffer
	observer := NewNDJSONObserver(&out, cliStaticClock{now: fixed})
	observer.OnEvent(context.Background(), engine.Event{
		Name:     engine.EventSyncStart,
		Database: "ai_pushka_biz",
		Tables:   42,
		Engine:   "native",
	})
	observer.OnEvent(context.Background(), engine.Event{
		Name:     engine.EventSchemaPreDataDone,
		Level:    "debug",
		Duration: 820 * time.Millisecond,
	})
	observer.OnEvent(context.Background(), engine.Event{
		Name:      engine.EventTableCopyStart,
		Time:      fixed.Add(time.Second),
		Table:     "messages",
		Estimated: 1_000_000,
	})
	observer.OnEvent(context.Background(), engine.Event{
		Name:        engine.EventTableCopyProgress,
		Table:       "messages",
		Rows:        120_000,
		Percent:     12,
		BytesPerSec: 42_000_000,
	})
	observer.OnEvent(context.Background(), engine.Event{
		Name:     engine.EventTableCopyDone,
		Table:    "messages",
		Rows:     1_000_000,
		Duration: 4231 * time.Millisecond,
	})
	observer.OnEvent(context.Background(), engine.Event{Name: engine.EventSchemaPostDataStart})
	observer.OnEvent(context.Background(), engine.Event{
		Name:     engine.EventSchemaPostDataDone,
		Duration: 2100 * time.Millisecond,
	})
	observer.OnEvent(context.Background(), engine.Event{
		Name:     engine.EventSyncDone,
		Tables:   42,
		Bytes:    665_000_000,
		Duration: 31420 * time.Millisecond,
	})
	sensitiveMessage := strings.Join([]string{
		"postgres://user:", "sec", "ret", "@db/app pass", "word=", "hunter", "2 sslmode=require",
	}, "")
	observer.OnEvent(context.Background(), engine.Event{
		Name:  engine.EventSyncFailed,
		Stage: "copy",
		Table: "messages",
		Error: sensitiveMessage,
	})

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.Len(t, lines, 9)

	start := decodeNDJSONLine(t, lines[0])
	assert.Equal(t, fixed.Format(time.RFC3339Nano), start["ts"])
	assert.Equal(t, "info", start["level"])
	assert.Equal(t, engine.EventSyncStart, start["event"])
	assert.Equal(t, "ai_pushka_biz", start["db"])
	assert.Equal(t, float64(42), start["tables"])
	assert.Equal(t, "native", start["engine"])

	preDataDone := decodeNDJSONLine(t, lines[1])
	assert.Equal(t, engine.EventSchemaPreDataDone, preDataDone["event"])
	assert.Equal(t, "debug", preDataDone["level"])
	assert.Equal(t, float64(820), preDataDone["duration_ms"])

	tableStart := decodeNDJSONLine(t, lines[2])
	assert.Equal(t, fixed.Add(time.Second).Format(time.RFC3339Nano), tableStart["ts"])
	assert.Equal(t, engine.EventTableCopyStart, tableStart["event"])
	assert.Equal(t, "messages", tableStart["table"])
	assert.Equal(t, float64(1_000_000), tableStart["est_rows"])

	progress := decodeNDJSONLine(t, lines[3])
	assert.Equal(t, engine.EventTableCopyProgress, progress["event"])
	assert.Equal(t, "messages", progress["table"])
	assert.Equal(t, float64(120_000), progress["rows"])
	assert.Equal(t, float64(12), progress["pct"])
	assert.Equal(t, float64(42_000_000), progress["bytes_per_sec"])

	tableDone := decodeNDJSONLine(t, lines[4])
	assert.Equal(t, engine.EventTableCopyDone, tableDone["event"])
	assert.Equal(t, float64(1_000_000), tableDone["rows"])
	assert.Equal(t, float64(4231), tableDone["duration_ms"])

	postDataStart := decodeNDJSONLine(t, lines[5])
	assert.Equal(t, engine.EventSchemaPostDataStart, postDataStart["event"])

	postDataDone := decodeNDJSONLine(t, lines[6])
	assert.Equal(t, engine.EventSchemaPostDataDone, postDataDone["event"])
	assert.Equal(t, float64(2100), postDataDone["duration_ms"])

	syncDone := decodeNDJSONLine(t, lines[7])
	assert.Equal(t, engine.EventSyncDone, syncDone["event"])
	assert.Equal(t, float64(31420), syncDone["duration_ms"])
	assert.Equal(t, float64(42), syncDone["tables"])
	assert.Equal(t, float64(665_000_000), syncDone["bytes"])

	failed := decodeNDJSONLine(t, lines[8])
	assert.Equal(t, engine.EventSyncFailed, failed["event"])
	assert.Equal(t, "error", failed["level"])
	assert.Equal(t, "copy", failed["stage"])
	assert.Equal(t, "messages", failed["table"])
	assert.Equal(t, "postgres://user:******@db/app password=****** sslmode=require", failed["error"])
	assert.NotContains(t, out.String(), "secret")
	assert.NotContains(t, out.String(), "hunter2")
}

func TestNDJSONObserverMapsNativeStageNamesAndFiltersUnknowns(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	observer := NewNDJSONObserver(&out, cliStaticClock{now: time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)})
	for _, name := range []string{
		"apply-pre-data.start",
		"apply-pre-data.done",
		"apply-post-data.start",
		"apply-post-data.done",
		"snapshot.start",
	} {
		observer.OnEvent(context.Background(), engine.Event{Name: name})
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.Len(t, lines, 4)
	assert.Equal(t, engine.EventSchemaPreDataStart, decodeNDJSONLine(t, lines[0])["event"])
	assert.Equal(t, engine.EventSchemaPreDataDone, decodeNDJSONLine(t, lines[1])["event"])
	assert.Equal(t, engine.EventSchemaPostDataStart, decodeNDJSONLine(t, lines[2])["event"])
	assert.Equal(t, engine.EventSchemaPostDataDone, decodeNDJSONLine(t, lines[3])["event"])
}

func TestNDJSONObserverNilReceiverAndDefaultsAreSafe(t *testing.T) {
	t.Parallel()
	var observer *NDJSONObserver
	observer.OnEvent(context.Background(), engine.Event{Name: engine.EventSyncStart})
	NewNDJSONObserver(nil, nil).OnEvent(context.Background(), engine.Event{Name: engine.EventSyncStart})
	assert.Equal(t, "password=****** postgres://u:******@h/db?pass=******", scrubSecrets("password=one postgres://u:two@h/db?pass=three"))
}

func decodeNDJSONLine(t *testing.T, line string) map[string]any {
	t.Helper()
	var record map[string]any
	require.NoError(t, json.Unmarshal([]byte(line), &record))
	return record
}
