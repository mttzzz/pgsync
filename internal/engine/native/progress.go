package native

import (
	"context"
	"io"
	"time"

	clockpkg "github.com/mttzzz/pgsync/internal/clock"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/models"
)

const copyStage = "copy"

// ProgressOptions configures a ProgressReader.
type ProgressOptions struct {
	Table          models.Table
	Observer       engine.ProgressObserver
	Clock          clockpkg.Clock
	Interval       time.Duration
	Estimated      int64
	EstimatedBytes int64
	EstimatedRows  int64
	Total          int64
	Context        context.Context
}

// ProgressReader wraps a reader and emits deterministic byte progress events.
type ProgressReader struct {
	r        io.Reader
	observer engine.ProgressObserver
	clock    clockpkg.Clock
	ctx      context.Context

	table     string
	estimated int64
	interval  time.Duration
	started   time.Time
	lastEmit  time.Time
	bytes     int64
	final     bool
}

// NewProgressReader returns an io.Reader wrapper that counts copied bytes and
// emits an initial progress event immediately.
func NewProgressReader(r io.Reader, opts ProgressOptions) *ProgressReader {
	progressClock := opts.Clock
	if progressClock == nil {
		progressClock = clockpkg.NewSystem()
	}
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	started := progressClock.Now()
	progress := &ProgressReader{
		r:         r,
		observer:  opts.Observer,
		clock:     progressClock,
		ctx:       ctx,
		table:     tableEventName(opts.Table),
		estimated: progressEstimate(opts),
		interval:  opts.Interval,
		started:   started,
		lastEmit:  started,
	}
	progress.emit(started)
	return progress
}

// Read reads from the wrapped reader, increments the byte counter, and emits
// throttled progress events. A final event is emitted when the wrapped reader
// reaches EOF or returns another terminal error.
func (p *ProgressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	if n > 0 {
		p.bytes += int64(n)
		p.emitIfDue(p.clock.Now())
	}
	if err != nil {
		p.emitFinal(p.clock.Now())
	}
	return n, err
}

// Bytes returns the number of bytes that have passed through the reader.
func (p *ProgressReader) Bytes() int64 {
	return p.bytes
}

func (p *ProgressReader) emitIfDue(now time.Time) {
	if p.interval > 0 && now.Sub(p.lastEmit) < p.interval {
		return
	}
	p.emit(now)
}

func (p *ProgressReader) emitFinal(now time.Time) {
	if p.final {
		return
	}
	p.final = true
	p.emit(now)
}

func (p *ProgressReader) emit(now time.Time) {
	p.lastEmit = now
	if p.observer == nil {
		return
	}
	elapsed := now.Sub(p.started)
	p.observer.OnEvent(p.ctx, engine.Event{
		Time:        now,
		Level:       "info",
		Name:        engine.EventTableCopyProgress,
		Stage:       copyStage,
		Engine:      string(engine.ModeNative),
		Table:       p.table,
		Estimated:   p.estimated,
		Bytes:       p.bytes,
		Percent:     progressPercent(p.bytes, p.estimated),
		BytesPerSec: progressBytesPerSecond(p.bytes, elapsed),
		Duration:    elapsed,
	})
}

func progressEstimate(opts ProgressOptions) int64 {
	estimates := []int64{opts.Estimated, opts.EstimatedBytes, opts.Total, opts.Table.SizeBytes, opts.EstimatedRows, opts.Table.Rows}
	for _, estimate := range estimates {
		if estimate > 0 {
			return estimate
		}
	}
	return 0
}

func progressPercent(done int64, estimated int64) float64 {
	if estimated <= 0 {
		return 0
	}
	percent := float64(done) / float64(estimated) * 100
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}

func progressBytesPerSecond(bytes int64, elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}
	return float64(bytes) / elapsed.Seconds()
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
