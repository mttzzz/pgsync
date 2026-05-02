package engine

import "time"

const (
	// EventSyncStart is emitted when a sync run starts.
	EventSyncStart = "sync.start"
	// EventSchemaPreDataStart is emitted before pre-data schema restore starts.
	EventSchemaPreDataStart = "schema.predata.start"
	// EventSchemaPreDataDone is emitted after pre-data schema restore completes.
	EventSchemaPreDataDone = "schema.predata.done"
	// EventTableCopyStart is emitted before copying one table starts.
	EventTableCopyStart = "table.copy.start"
	// EventTableCopyProgress is emitted as table copy progress advances.
	EventTableCopyProgress = "table.copy.progress"
	// EventTableCopyDone is emitted after copying one table completes.
	EventTableCopyDone = "table.copy.done"
	// EventSchemaPostDataStart is emitted before post-data schema restore starts.
	EventSchemaPostDataStart = "schema.postdata.start"
	// EventSchemaPostDataDone is emitted after post-data schema restore completes.
	EventSchemaPostDataDone = "schema.postdata.done"
	// EventSyncDone is emitted after a sync run completes successfully.
	EventSyncDone = "sync.done"
	// EventSyncFailed is emitted after a sync run fails.
	EventSyncFailed = "sync.failed"
)

// Event describes one engine progress or lifecycle event.
type Event struct {
	Time        time.Time
	Level       string
	Name        string
	Stage       string
	Database    string
	Engine      string
	Table       string
	Tables      int
	Rows        int64
	Estimated   int64
	Bytes       int64
	Percent     float64
	BytesPerSec float64
	Duration    time.Duration
	Error       string
}
