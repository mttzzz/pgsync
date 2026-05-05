package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/tui/screens"
	"github.com/mttzzz/pgsync/internal/tui/ui"
)

// LiveProgress tracks three tiers of state for the live sync dashboard:
// queue (whole run), current DB, and current table. Counters are split into
// completed (done tables) and current (in-flight) so the displayed totals
// move smoothly without double-counting.
type LiveProgress struct {
	StartedAt time.Time
	Now       time.Time
	Stage     string
	Errors    int

	// queue (across all selected DBs in this run)
	DBIndex             int
	DBTotal             int
	QueueBytesEstimated int64
	QueueRowsEstimated  int64
	QueueTablesTotal    int
	QueueTablesDone     int
	queueCompletedRows  int64
	queueCompletedBytes int64

	// current DB
	CurrentDatabase  string
	DBBytesEstimated int64
	DBRowsEstimated  int64
	DBTablesTotal    int
	DBTablesDone     int
	dbCompletedRows  int64
	dbCompletedBytes int64

	// current table (in flight)
	CurrentTable         string
	CurrentStartedAt     time.Time
	CurrentRows          int64
	CurrentRowsEstimate  int64
	CurrentBytes         int64
	CurrentBytesEstimate int64
	CurrentPercent       float64
	CurrentBytesPerSec   float64

	// animation (eased monotone, no overshoot)
	queueAnim float64
	dbAnim    float64

	Events       []screens.ProgressEventRow
	TableResults []screens.TableResultRow
	planTables   map[string]models.Table
	seenDBs      map[string]bool
}

// NewLiveProgress initializes from a single sync plan (single-DB callers / tests).
func NewLiveProgress(plan *models.SyncPlan, now time.Time) LiveProgress {
	progress := LiveProgress{StartedAt: now, Now: now, planTables: map[string]models.Table{}, seenDBs: map[string]bool{}}
	if plan == nil {
		return progress
	}
	for _, table := range plan.Tables {
		key := tableEventName(table)
		progress.planTables[key] = table
		progress.QueueRowsEstimated += table.Rows
		progress.QueueBytesEstimated += table.SizeBytes
		progress.DBRowsEstimated += table.Rows
		progress.DBBytesEstimated += table.SizeBytes
	}
	progress.QueueTablesTotal = len(plan.Tables)
	progress.DBTablesTotal = len(plan.Tables)
	if plan.Database != "" {
		progress.CurrentDatabase = plan.Database
	}
	return progress
}

// NewLiveProgressForQueue initializes for a multi-DB queue. Byte/table totals
// come from the catalog; row totals stay zero until each DB plan is registered.
func NewLiveProgressForQueue(queue []models.Database, now time.Time) LiveProgress {
	progress := LiveProgress{StartedAt: now, Now: now, planTables: map[string]models.Table{}, seenDBs: map[string]bool{}, DBTotal: len(queue)}
	for _, db := range queue {
		progress.QueueBytesEstimated += db.SizeBytes
		progress.QueueTablesTotal += db.TableCount
	}
	return progress
}

// RegisterDB attaches a fresh DB plan to live progress. Called when planner
// returns a plan for the next queued DB, before executor.Execute starts.
func (p *LiveProgress) RegisterDB(database string, tables []models.Table, rows, bytes int64) {
	p.CurrentDatabase = database
	p.DBBytesEstimated = bytes
	p.DBRowsEstimated = rows
	p.DBTablesTotal = len(tables)
	p.DBTablesDone = 0
	p.dbCompletedRows = 0
	p.dbCompletedBytes = 0
	p.CurrentTable = ""
	p.CurrentBytes = 0
	p.CurrentRows = 0
	p.CurrentBytesEstimate = 0
	p.CurrentRowsEstimate = 0
	p.CurrentPercent = 0
	p.dbAnim = 0
	p.QueueRowsEstimated += rows
	p.planTables = map[string]models.Table{}
	for _, table := range tables {
		p.planTables[tableEventName(table)] = table
	}
	if p.seenDBs == nil {
		p.seenDBs = map[string]bool{}
	}
	if !p.seenDBs[database] {
		p.seenDBs[database] = true
		p.DBIndex = len(p.seenDBs)
		if p.DBTotal > 0 && p.DBIndex > p.DBTotal {
			p.DBTotal = p.DBIndex
		}
	}
}

// Apply records a new engine event and updates derived counters.
//
//nolint:gocyclo,gocognit // Event aggregation is one explicit branch per event type.
func (p *LiveProgress) Apply(event engine.Event, now time.Time) {
	if p.StartedAt.IsZero() {
		p.StartedAt = now
	}
	p.Now = now
	if event.Time.IsZero() {
		event.Time = now
	}
	if event.Name != "" {
		p.Stage = eventStageLabel(event)
	}
	if event.BytesPerSec > 0 {
		p.CurrentBytesPerSec = event.BytesPerSec
	}
	if event.Database != "" && event.Database != p.CurrentDatabase {
		// EventSyncStart for a new DB without prior RegisterDB (e.g. tests, or
		// engine emitted before plan-ready landed). Track DB index, reset table.
		if p.seenDBs == nil {
			p.seenDBs = map[string]bool{}
		}
		if !p.seenDBs[event.Database] {
			p.seenDBs[event.Database] = true
			p.DBIndex = len(p.seenDBs)
			if p.DBTotal > 0 && p.DBIndex > p.DBTotal {
				p.DBTotal = p.DBIndex
			}
		}
		p.CurrentDatabase = event.Database
		p.CurrentTable = ""
		p.CurrentBytes = 0
		p.CurrentRows = 0
		p.CurrentBytesEstimate = 0
		p.CurrentRowsEstimate = 0
		p.CurrentPercent = 0
	}

	switch event.Name {
	case engine.EventTableCopyStart:
		p.CurrentTable = event.Table
		p.CurrentStartedAt = event.Time
		p.CurrentRows = 0
		p.CurrentBytes = 0
		p.CurrentPercent = 0
		p.applyTableEstimate(event)
	case engine.EventTableCopyProgress:
		p.CurrentTable = event.Table
		p.CurrentBytes = event.Bytes
		p.applyTableEstimate(event)
		p.CurrentPercent = event.Percent
	case engine.EventTableCopyDone:
		p.CurrentTable = event.Table
		p.CurrentRows = event.Rows
		p.CurrentBytes = event.Bytes
		p.applyTableEstimate(event)
		p.CurrentPercent = 100
		p.queueCompletedRows += event.Rows
		p.queueCompletedBytes += event.Bytes
		p.dbCompletedRows += event.Rows
		p.dbCompletedBytes += event.Bytes
		p.QueueTablesDone++
		p.DBTablesDone++
		p.recordTableResult(event)
		// reset in-flight after committing — next Start fills again
		p.CurrentTable = ""
		p.CurrentBytes = 0
		p.CurrentRows = 0
		p.CurrentBytesEstimate = 0
		p.CurrentRowsEstimate = 0
		p.CurrentPercent = 0
	case engine.EventSyncFailed:
		p.Errors++
	}

	p.queueAnim = easeToward(p.queueAnim, p.QueuePercent())
	p.dbAnim = easeToward(p.dbAnim, p.DBPercent())
	p.prependEvent(event)
}

// Tick advances eased animation with no new event.
func (p *LiveProgress) Tick(now time.Time) {
	if p.StartedAt.IsZero() {
		p.StartedAt = now
	}
	p.Now = now
	p.queueAnim = easeToward(p.queueAnim, p.QueuePercent())
	p.dbAnim = easeToward(p.dbAnim, p.DBPercent())
}

// QueueBytesCopied returns committed + in-flight bytes across the queue.
func (p LiveProgress) QueueBytesCopied() int64 { return p.queueCompletedBytes + p.CurrentBytes }

// QueueRowsCopied returns committed rows; in-flight rows are unknown until done.
func (p LiveProgress) QueueRowsCopied() int64 { return p.queueCompletedRows }

// DBBytesCopied returns committed + in-flight bytes for the current DB.
func (p LiveProgress) DBBytesCopied() int64 { return p.dbCompletedBytes + p.CurrentBytes }

// DBRowsCopied returns committed rows for the current DB.
func (p LiveProgress) DBRowsCopied() int64 { return p.dbCompletedRows }

// QueuePercent returns 0..100 byte progress across the queue.
func (p LiveProgress) QueuePercent() float64 {
	return safePercent(p.QueueBytesCopied(), p.QueueBytesEstimated)
}

// DBPercent returns 0..100 byte progress for the current DB.
func (p LiveProgress) DBPercent() float64 {
	return safePercent(p.DBBytesCopied(), p.DBBytesEstimated)
}

// Snapshot converts aggregated state to the screen renderer DTO.
func (p LiveProgress) Snapshot(cfg config.Config, width int) screens.ProgressSnapshot {
	database := p.CurrentDatabase
	if database == "" {
		database = cfg.Runtime.DefaultDatabase
	}
	return screens.ProgressSnapshot{
		Header:               screens.HeaderOptions{Config: cfg, Database: database, Width: width, Running: true, Started: p.StartedAt, Now: p.Now},
		Stage:                p.Stage,
		StartedAt:            p.StartedAt,
		Now:                  p.Now,
		DBIndex:              p.DBIndex,
		DBTotal:              p.DBTotal,
		CurrentDatabase:      p.CurrentDatabase,
		QueueBytesCopied:     p.QueueBytesCopied(),
		QueueBytesEstimated:  p.QueueBytesEstimated,
		QueueRowsCopied:      p.QueueRowsCopied(),
		QueueRowsEstimated:   p.QueueRowsEstimated,
		QueueTablesDone:      p.QueueTablesDone,
		QueueTablesTotal:     p.QueueTablesTotal,
		QueuePercent:         p.QueuePercent(),
		QueueAnimatedPercent: p.queueAnim,
		DBBytesCopied:        p.DBBytesCopied(),
		DBBytesEstimated:     p.DBBytesEstimated,
		DBRowsCopied:         p.DBRowsCopied(),
		DBRowsEstimated:      p.DBRowsEstimated,
		DBTablesDone:         p.DBTablesDone,
		DBTablesTotal:        p.DBTablesTotal,
		DBPercent:            p.DBPercent(),
		DBAnimatedPercent:    p.dbAnim,
		CurrentTable:         p.CurrentTable,
		CurrentStartedAt:     p.CurrentStartedAt,
		CurrentRows:          p.CurrentRows,
		CurrentRowsEstimate:  p.CurrentRowsEstimate,
		CurrentBytes:         p.CurrentBytes,
		CurrentBytesEstimate: p.CurrentBytesEstimate,
		CurrentPercent:       p.CurrentPercent,
		BytesPerSec:          p.CurrentBytesPerSec,
		Errors:               p.Errors,
		Events:               p.Events,
	}
}

func (p *LiveProgress) applyTableEstimate(event engine.Event) {
	if table, ok := p.planTables[event.Table]; ok {
		p.CurrentRowsEstimate = table.Rows
		p.CurrentBytesEstimate = table.SizeBytes
		return
	}
	if event.Estimated > 0 {
		p.CurrentBytesEstimate = event.Estimated
	}
}

func (p *LiveProgress) prependEvent(event engine.Event) {
	row := screens.ProgressEventRow{Time: event.Time, Level: emptyValue(event.Level, "info"), Event: event.Name, Table: event.Table, Details: eventDetails(event)}
	p.Events = append([]screens.ProgressEventRow{row}, p.Events...)
	if len(p.Events) > 128 {
		p.Events = p.Events[:128]
	}
}

func (p *LiveProgress) recordTableResult(event engine.Event) {
	speed := 0.0
	if event.Duration > 0 {
		speed = float64(event.Bytes) / event.Duration.Seconds()
	}
	database := event.Database
	if database == "" {
		database = p.CurrentDatabase
	}
	p.TableResults = append(p.TableResults, screens.TableResultRow{Database: database, Table: event.Table, Rows: event.Rows, Bytes: event.Bytes, Duration: event.Duration, Speed: speed})
}

// easeToward moves current toward target with a fixed exponential ratio,
// monotone so the bar never overshoots.
func easeToward(current, target float64) float64 {
	if target <= current {
		// snap down (estimates revising) to keep the bar truthful
		return target
	}
	const stepRatio = 0.25
	return current + (target-current)*stepRatio
}

func safePercent(numerator, denominator int64) float64 {
	if denominator <= 0 {
		return 0
	}
	pct := float64(numerator) / float64(denominator) * 100
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

func eventStageLabel(event engine.Event) string {
	if event.Stage != "" {
		return event.Stage
	}
	if event.Name != "" {
		return event.Name
	}
	return "waiting"
}

func eventDetails(event engine.Event) string {
	switch event.Name {
	case engine.EventTableCopyStart:
		return "disk est " + ui.FormatBytes(event.Estimated)
	case engine.EventTableCopyProgress:
		return fmt.Sprintf("%s COPY stream, %s of disk est, %s", ui.FormatBytes(event.Bytes), ui.FormatPercent(event.Percent), ui.FormatBytesRate(event.BytesPerSec))
	case engine.EventTableCopyDone:
		return fmt.Sprintf("%s rows, %s COPY stream, %s", ui.FormatInt(event.Rows), ui.FormatBytes(event.Bytes), ui.FormatDurationTenths(event.Duration))
	case engine.EventSchemaPreDataDone, engine.EventSchemaPostDataDone:
		return ui.FormatDurationTenths(event.Duration)
	case engine.EventSyncFailed:
		return event.Error
	default:
		if event.Error != "" {
			return event.Error
		}
		if event.Duration > 0 {
			return ui.FormatDurationTenths(event.Duration)
		}
		return ""
	}
}

func tableEventName(table models.Table) string {
	if table.Schema == "" {
		return table.Name
	}
	if table.Name == "" {
		return table.Schema
	}
	return table.Schema + "." + table.Name
}

func emptyValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
