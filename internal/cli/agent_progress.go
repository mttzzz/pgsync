package cli

import (
	"context"
	"encoding/json"
	"io"
	"regexp"
	"sync"
	"time"

	clockpkg "github.com/mttzzz/pgsync/internal/clock"
	"github.com/mttzzz/pgsync/internal/engine"
)

var (
	passwordKeywordPattern = regexp.MustCompile(`(?i)(password=)([^\s;]+)`)
	passwordQueryPattern   = regexp.MustCompile(`(?i)([?&](?:password|pass)=)([^&\s]+)`)
	postgresURLPattern     = regexp.MustCompile(`(?i)(postgres(?:ql)?://[^:\s/@]+:)([^@\s]+)(@)`)
)

// NDJSONObserver writes one JSON progress event per line for automation.
type NDJSONObserver struct {
	encoder *json.Encoder
	clock   clockpkg.Clock
	mu      sync.Mutex
}

// NewNDJSONObserver returns an NDJSON progress observer.
func NewNDJSONObserver(w io.Writer, clock clockpkg.Clock) *NDJSONObserver {
	if clock == nil {
		clock = clockpkg.NewSystem()
	}
	return &NDJSONObserver{encoder: json.NewEncoder(outputWriter(w)), clock: clock}
}

// OnEvent writes a spec-compatible NDJSON line for one engine event.
func (o *NDJSONObserver) OnEvent(_ context.Context, event engine.Event) {
	if o == nil {
		return
	}
	name, ok := specEventName(event.Name)
	if !ok {
		return
	}
	record := o.ndjsonRecord(name, event)
	o.mu.Lock()
	defer o.mu.Unlock()
	_ = o.encoder.Encode(record)
}

func (o *NDJSONObserver) ndjsonRecord(name string, event engine.Event) map[string]any {
	record := map[string]any{
		"ts":    eventTimestamp(event.Time, o.clock),
		"level": ndjsonLevel(name, event.Level),
		"event": name,
	}
	addCommonNDJSONFields(record, name, event)
	addEventNDJSONFields(record, name, event)
	return record
}

func specEventName(name string) (string, bool) {
	switch name {
	case engine.EventSyncStart,
		engine.EventSchemaPreDataStart,
		engine.EventSchemaPreDataDone,
		engine.EventTableCopyStart,
		engine.EventTableCopyProgress,
		engine.EventTableCopyDone,
		engine.EventSchemaPostDataStart,
		engine.EventSchemaPostDataDone,
		engine.EventSyncDone,
		engine.EventSyncFailed:
		return name, true
	case "apply-pre-data.start":
		return engine.EventSchemaPreDataStart, true
	case "apply-pre-data.done":
		return engine.EventSchemaPreDataDone, true
	case "apply-post-data.start":
		return engine.EventSchemaPostDataStart, true
	case "apply-post-data.done":
		return engine.EventSchemaPostDataDone, true
	default:
		return "", false
	}
}

func addCommonNDJSONFields(record map[string]any, name string, event engine.Event) {
	if name == engine.EventSyncStart || name == engine.EventSyncDone {
		record["tables"] = event.Tables
	}
	putString(record, "db", event.Database)
	putString(record, "engine", event.Engine)
}

func addEventNDJSONFields(record map[string]any, name string, event engine.Event) {
	switch name {
	case engine.EventSchemaPreDataDone, engine.EventSchemaPostDataDone:
		record["duration_ms"] = event.Duration.Milliseconds()
	case engine.EventTableCopyStart:
		putString(record, "table", event.Table)
		record["disk_bytes_est"] = event.Estimated
	case engine.EventTableCopyProgress:
		putString(record, "table", event.Table)
		record["copy_stream_bytes"] = event.Bytes
		record["pct_of_disk_est"] = event.Percent
		record["bytes_per_sec"] = event.BytesPerSec
	case engine.EventTableCopyDone:
		putString(record, "table", event.Table)
		record["rows"] = event.Rows
		record["copy_stream_bytes"] = event.Bytes
		record["duration_ms"] = event.Duration.Milliseconds()
	case engine.EventSyncDone:
		record["duration_ms"] = event.Duration.Milliseconds()
		record["copy_stream_bytes"] = event.Bytes
	case engine.EventSyncFailed:
		putString(record, "stage", event.Stage)
		putString(record, "table", event.Table)
		putString(record, "error", event.Error)
	}
}

func eventTimestamp(eventTime time.Time, clock clockpkg.Clock) string {
	if eventTime.IsZero() {
		eventTime = clock.Now()
	}
	return eventTime.UTC().Format(time.RFC3339Nano)
}

func ndjsonLevel(name string, level string) string {
	if name == engine.EventSyncFailed {
		return "error"
	}
	if level == "" {
		return "info"
	}
	return scrubSecrets(level)
}

func putString(record map[string]any, key string, value string) {
	if value != "" {
		record[key] = scrubSecrets(value)
	}
}

func scrubSecrets(text string) string {
	text = postgresURLPattern.ReplaceAllString(text, "${1}******${3}")
	text = passwordKeywordPattern.ReplaceAllString(text, "${1}******")
	return passwordQueryPattern.ReplaceAllString(text, "${1}******")
}
