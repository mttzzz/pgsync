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

// LiveProgress stores aggregated state for the live sync dashboard.
type LiveProgress struct {
	StartedAt            time.Time
	Now                  time.Time
	CurrentTable         string
	CurrentDatabase      string
	CurrentStartedAt     time.Time
	Stage                string
	DBIndex              int
	DBTotal              int
	TablesDone           int
	TablesTotal          int
	RowsCopied           int64
	RowsEstimated        int64
	BytesCopied          int64
	BytesEstimated       int64
	BytesPerSec          float64
	CompletedRows        int64
	CompletedBytes       int64
	CurrentRows          int64
	CurrentBytes         int64
	CurrentRowsEstimate  int64
	CurrentBytesEstimate int64
	TablePercent         float64
	OverallPercent       float64
	AnimatedPercent      float64
	ProgressVelocity     float64
	Errors               int
	Events               []screens.ProgressEventRow
	TableResults         []screens.TableResultRow
	planTables           map[string]models.Table
	seenDatabases        map[string]bool
}

// NewLiveProgress initializes progress aggregation from a single sync plan.
func NewLiveProgress(plan *models.SyncPlan, now time.Time) LiveProgress {
	progress := LiveProgress{StartedAt: now, Now: now, planTables: map[string]models.Table{}, seenDatabases: map[string]bool{}}
	if plan == nil {
		return progress
	}
	progress.TablesTotal = len(plan.Tables)
	for _, table := range plan.Tables {
		key := tableEventName(table)
		progress.planTables[key] = table
		progress.RowsEstimated += table.Rows
		progress.BytesEstimated += table.SizeBytes
	}
	return progress
}

// NewLiveProgressForQueue initializes progress aggregation for a multi-DB queue.
// TablesTotal/BytesEstimated come from the catalog DB stats since per-table plans
// are built lazily during execution.
func NewLiveProgressForQueue(queue []models.Database, now time.Time) LiveProgress {
	progress := LiveProgress{StartedAt: now, Now: now, planTables: map[string]models.Table{}, seenDatabases: map[string]bool{}, DBTotal: len(queue)}
	for _, db := range queue {
		progress.TablesTotal += db.TableCount
		progress.BytesEstimated += db.SizeBytes
	}
	return progress
}

// Apply records a new engine event and updates derived metrics.
//
//nolint:gocyclo,gocognit // Event aggregation has one explicit branch per engine event type plus DB-switch reset.
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
	if event.Tables > 0 && p.TablesTotal == 0 {
		p.TablesTotal = event.Tables
	}
	if event.BytesPerSec > 0 {
		p.BytesPerSec = event.BytesPerSec
	}
	if event.Database != "" && event.Database != p.CurrentDatabase {
		if p.seenDatabases == nil {
			p.seenDatabases = map[string]bool{}
		}
		if !p.seenDatabases[event.Database] {
			p.seenDatabases[event.Database] = true
			p.DBIndex = len(p.seenDatabases)
			if p.DBTotal > 0 && p.DBIndex > p.DBTotal {
				p.DBTotal = p.DBIndex
			}
		}
		p.CurrentDatabase = event.Database
		p.CurrentTable = ""
		p.TablePercent = 0
		p.CurrentRows = 0
		p.CurrentBytes = 0
		p.CurrentRowsEstimate = 0
		p.CurrentBytesEstimate = 0
	}

	switch event.Name {
	case engine.EventTableCopyStart:
		p.CurrentTable = event.Table
		p.CurrentStartedAt = event.Time
		p.CurrentRows = 0
		p.CurrentBytes = 0
		p.TablePercent = 0
		p.applyTableEstimate(event)
	case engine.EventTableCopyProgress:
		p.CurrentTable = event.Table
		// Native progress events are byte-based; final done events carry real row counts.
		p.CurrentBytes = event.Bytes
		p.applyTableEstimate(event)
		p.TablePercent = event.Percent
	case engine.EventTableCopyDone:
		p.CurrentTable = event.Table
		p.CurrentRows = event.Rows
		p.CurrentBytes = event.Bytes
		p.applyTableEstimate(event)
		p.TablePercent = 100
		p.TablesDone++
		p.CompletedRows += event.Rows
		p.CompletedBytes += event.Bytes
		p.RowsCopied = p.CompletedRows
		p.BytesCopied = p.CompletedBytes
		p.recordTableResult(event)
	case engine.EventSyncFailed:
		p.Errors++
	}
	if event.Name != engine.EventTableCopyDone {
		p.RowsCopied = p.CompletedRows
		p.BytesCopied = p.CompletedBytes + p.CurrentBytes
	}
	p.recalculateOverall()
	p.AnimatedPercent, p.ProgressVelocity = ui.SmoothProgress(p.AnimatedPercent, p.ProgressVelocity, p.OverallPercent)
	p.prependEvent(event)
}

// Tick advances timers and smooth progress without a new engine event.
func (p *LiveProgress) Tick(now time.Time) {
	if p.StartedAt.IsZero() {
		p.StartedAt = now
	}
	p.Now = now
	p.AnimatedPercent, p.ProgressVelocity = ui.SmoothProgress(p.AnimatedPercent, p.ProgressVelocity, p.OverallPercent)
}

// Snapshot converts aggregate state to the screen renderer DTO.
func (p LiveProgress) Snapshot(cfg config.Config, width int) screens.ProgressSnapshot {
	database := p.CurrentDatabase
	if database == "" {
		database = cfg.Runtime.DefaultDatabase
	}
	return screens.ProgressSnapshot{
		Header:             screens.HeaderOptions{Config: cfg, Database: database, Width: width, Running: true, Started: p.StartedAt, Now: p.Now},
		Stage:              p.Stage,
		CurrentTable:       p.CurrentTable,
		StartedAt:          p.StartedAt,
		CurrentStartedAt:   p.CurrentStartedAt,
		Now:                p.Now,
		DBIndex:            p.DBIndex,
		DBTotal:            p.DBTotal,
		CurrentDatabase:    p.CurrentDatabase,
		TablesDone:         p.TablesDone,
		TablesTotal:        p.TablesTotal,
		RowsCopied:         p.RowsCopied,
		RowsEstimated:      p.RowsEstimated,
		BytesCopied:        p.BytesCopied,
		BytesEstimated:     p.BytesEstimated,
		BytesPerSec:        p.BytesPerSec,
		TableRows:          p.CurrentRows,
		TableRowsEstimate:  p.CurrentRowsEstimate,
		TableBytes:         p.CurrentBytes,
		TableBytesEstimate: p.CurrentBytesEstimate,
		TablePercent:       p.TablePercent,
		OverallPercent:     p.OverallPercent,
		AnimatedPercent:    p.AnimatedPercent,
		Errors:             p.Errors,
		Events:             p.Events,
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

func (p *LiveProgress) recalculateOverall() {
	switch {
	case p.BytesEstimated > 0:
		p.OverallPercent = float64(p.BytesCopied) / float64(p.BytesEstimated) * 100
	case p.TablesTotal > 0:
		partial := p.TablePercent / 100
		p.OverallPercent = (float64(p.TablesDone) + partial) / float64(p.TablesTotal) * 100
	default:
		p.OverallPercent = p.TablePercent
	}
	if p.OverallPercent > 100 {
		p.OverallPercent = 100
	}
	if p.OverallPercent < 0 {
		p.OverallPercent = 0
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
	p.TableResults = append([]screens.TableResultRow{{Table: event.Table, Rows: event.Rows, Bytes: event.Bytes, Duration: event.Duration, Speed: speed}}, p.TableResults...)
	if len(p.TableResults) > 10 {
		p.TableResults = p.TableResults[:10]
	}
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
