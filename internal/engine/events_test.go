package engine_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mttzzz/pgsync/internal/engine"
)

func TestEventNamesMatchSpec(t *testing.T) {
	t.Parallel()
	want := []string{
		"sync.start",
		"schema.predata.start",
		"schema.predata.done",
		"table.copy.start",
		"table.copy.progress",
		"table.copy.done",
		"schema.postdata.start",
		"schema.postdata.done",
		"sync.done",
		"sync.failed",
	}
	got := []string{
		engine.EventSyncStart,
		engine.EventSchemaPreDataStart,
		engine.EventSchemaPreDataDone,
		engine.EventTableCopyStart,
		engine.EventTableCopyProgress,
		engine.EventTableCopyDone,
		engine.EventSchemaPostDataStart,
		engine.EventSchemaPostDataDone,
		engine.EventSyncDone,
		engine.EventSyncFailed,
	}

	assert.Equal(t, want, got)
	assert.Len(t, uniqueStrings(got), len(got))
}

func TestEventCarriesSafeProgressFields(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	event := engine.Event{
		Time:        now,
		Level:       "info",
		Name:        engine.EventTableCopyProgress,
		Stage:       "copy",
		Database:    "app",
		Engine:      string(engine.ModeNative),
		Table:       "public.users",
		Tables:      3,
		Rows:        10,
		Estimated:   100,
		Bytes:       2048,
		Percent:     10.0,
		BytesPerSec: 512.5,
		Duration:    2 * time.Second,
		Error:       "",
	}

	assert.Equal(t, now, event.Time)
	assert.Equal(t, "info", event.Level)
	assert.Equal(t, engine.EventTableCopyProgress, event.Name)
	assert.Equal(t, "copy", event.Stage)
	assert.Equal(t, "app", event.Database)
	assert.Equal(t, string(engine.ModeNative), event.Engine)
	assert.Equal(t, "public.users", event.Table)
	assert.Equal(t, 3, event.Tables)
	assert.Equal(t, int64(10), event.Rows)
	assert.Equal(t, int64(100), event.Estimated)
	assert.Equal(t, int64(2048), event.Bytes)
	assert.Equal(t, 10.0, event.Percent)
	assert.Equal(t, 512.5, event.BytesPerSec)
	assert.Equal(t, 2*time.Second, event.Duration)
	assert.Empty(t, event.Error)
}

func uniqueStrings(values []string) map[string]struct{} {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		unique[value] = struct{}{}
	}
	return unique
}
