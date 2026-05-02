package native

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	clockpkg "github.com/mttzzz/pgsync/internal/clock"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/pgdb"
)

const copyProgressInterval = time.Second

// CopyTableOptions contains all collaborators needed to COPY one table.
type CopyTableOptions struct {
	Table      models.Table
	SnapshotID string
	Source     pgdb.CopyConn
	Target     pgdb.CopyConn
	Observer   engine.ProgressObserver
	Clock      clockpkg.Clock
}

// CopyTable streams one table from source to target using PostgreSQL binary COPY.
func CopyTable(ctx context.Context, opts CopyTableOptions) (models.SyncResult, error) {
	if opts.Source == nil {
		return models.SyncResult{}, errors.New("source connection is required")
	}
	if opts.Target == nil {
		return models.SyncResult{}, errors.New("target connection is required")
	}

	quotedTable, err := pgdb.QuoteQualified(opts.Table)
	if err != nil {
		return models.SyncResult{}, err
	}

	copyClock := opts.Clock
	if copyClock == nil {
		copyClock = clockpkg.NewSystem()
	}
	started := copyClock.Now()
	emitCopyEvent(ctx, opts.Observer, engine.EventTableCopyStart, opts.Table, 0, 0, started, started)

	pipeReader, pipeWriter := io.Pipe()
	copyCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sourceDone := make(chan error, 1)
	targetDone := make(chan copyTargetResult, 1)

	go copyTableSource(copyCtx, copyTableSourceOptions{
		conn:       opts.Source,
		tableSQL:   quotedTable,
		snapshotID: opts.SnapshotID,
		writer:     pipeWriter,
		done:       sourceDone,
	})
	go copyTableTarget(copyCtx, copyTableTargetOptions{
		conn:     opts.Target,
		table:    opts.Table,
		tableSQL: quotedTable,
		reader:   pipeReader,
		observer: opts.Observer,
		clock:    copyClock,
		done:     targetDone,
	})

	targetResult := <-targetDone
	if targetResult.err != nil {
		cancel()
		_ = pipeReader.CloseWithError(targetResult.err)
	}
	sourceErr := <-sourceDone
	finished := copyClock.Now()

	if targetResult.err != nil {
		return copyTableFailed(started, finished, targetResult.reader.Bytes(), targetResult.rows, targetResult.err), targetResult.err
	}
	if sourceErr != nil {
		return copyTableFailed(started, finished, targetResult.reader.Bytes(), targetResult.rows, sourceErr), sourceErr
	}

	emitCopyEvent(ctx, opts.Observer, engine.EventTableCopyDone, opts.Table, targetResult.reader.Bytes(), targetResult.rows, started, finished)
	return models.SyncResult{
		StartedAt:    started,
		FinishedAt:   finished,
		BytesCopied:  targetResult.reader.Bytes(),
		RowsCopied:   targetResult.rows,
		TablesCopied: 1,
		Stages:       copyStages(started, finished),
	}, nil
}

// CopyTablesOptions configures bounded concurrent table copying.
type CopyTablesOptions struct {
	Tables        []models.Table
	SnapshotID    string
	Threads       int
	SourceFactory func(ctx context.Context) (pgdb.CopyConn, error)
	TargetFactory func(ctx context.Context) (pgdb.CopyConn, error)
	Observer      engine.ProgressObserver
	Clock         clockpkg.Clock
}

// CopyTables copies all tables with a bounded worker pool and returns an
// aggregate result. Work is cancelled after the first table or connection error.
func CopyTables(ctx context.Context, opts CopyTablesOptions) (*models.SyncResult, error) {
	copyClock := opts.Clock
	if copyClock == nil {
		copyClock = clockpkg.NewSystem()
	}
	started := copyClock.Now()
	if len(opts.Tables) == 0 {
		return &models.SyncResult{StartedAt: started, FinishedAt: copyClock.Now(), Stages: copyStages(started, started)}, nil
	}
	if opts.SourceFactory == nil {
		return nil, errors.New("source factory is required")
	}
	if opts.TargetFactory == nil {
		return nil, errors.New("target factory is required")
	}

	copyCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	results, err := runCopyWorkers(copyCtx, cancel, opts, workerCount(opts.Threads))
	finished := copyClock.Now()
	if err != nil {
		return aggregateCopyResults(started, finished, results, err), err
	}
	return aggregateCopyResults(started, finished, results, nil), nil
}

type copyTargetResult struct {
	reader *ProgressReader
	rows   int64
	err    error
}

type copyTableSourceOptions struct {
	conn       pgdb.CopyConn
	tableSQL   string
	snapshotID string
	writer     *io.PipeWriter
	done       chan<- error
}

func copyTableSource(ctx context.Context, opts copyTableSourceOptions) {
	copyErr := ApplySnapshot(ctx, opts.conn, opts.snapshotID)
	if copyErr == nil {
		_, copyErr = opts.conn.CopyTo(ctx, opts.writer, copyToSQL(opts.tableSQL))
	}
	closeErr := opts.writer.CloseWithError(copyErr)
	cleanupErr := rollbackAndClose(context.WithoutCancel(ctx), opts.conn)
	opts.done <- errors.Join(copyErr, closeErr, cleanupErr)
}

type copyTableTargetOptions struct {
	conn     pgdb.CopyConn
	table    models.Table
	tableSQL string
	reader   *io.PipeReader
	observer engine.ProgressObserver
	clock    clockpkg.Clock
	done     chan<- copyTargetResult
}

func copyTableTarget(ctx context.Context, opts copyTableTargetOptions) {
	progress := NewProgressReader(opts.reader, ProgressOptions{
		Table:     opts.table,
		Observer:  opts.observer,
		Clock:     opts.clock,
		Interval:  copyProgressInterval,
		Context:   ctx,
		Estimated: opts.table.SizeBytes,
	})
	tag, err := opts.conn.CopyFrom(ctx, progress, copyFromSQL(opts.tableSQL))
	if err != nil {
		_ = opts.reader.CloseWithError(err)
	}
	opts.done <- copyTargetResult{reader: progress, rows: rowsAffected(tag), err: err}
}

type copyJob struct {
	index int
	table models.Table
}

type copyWorkerResult struct {
	index  int
	result models.SyncResult
	err    error
}

func runCopyWorkers(
	ctx context.Context,
	cancel context.CancelFunc,
	opts CopyTablesOptions,
	workers int,
) ([]models.SyncResult, error) {
	jobs := make(chan copyJob)
	workerResults := make(chan copyWorkerResult, len(opts.Tables))
	var wg sync.WaitGroup
	for workerID := 0; workerID < workers; workerID++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			copyWorker(ctx, cancel, opts, jobs, workerResults)
		}()
	}

	go enqueueCopyJobs(ctx, opts.Tables, jobs)
	go closeCopyResults(&wg, workerResults)

	results := make([]models.SyncResult, len(opts.Tables))
	var firstErr error
	for workerResult := range workerResults {
		results[workerResult.index] = workerResult.result
		if workerResult.err != nil && firstErr == nil {
			firstErr = workerResult.err
			cancel()
		}
	}
	return results, firstErr
}

func enqueueCopyJobs(ctx context.Context, tables []models.Table, jobs chan<- copyJob) {
	defer close(jobs)
	for index, table := range tables {
		select {
		case <-ctx.Done():
			return
		case jobs <- copyJob{index: index, table: table}:
		}
	}
}

func closeCopyResults(wg *sync.WaitGroup, workerResults chan<- copyWorkerResult) {
	wg.Wait()
	close(workerResults)
}

func copyWorker(
	ctx context.Context,
	cancel context.CancelFunc,
	opts CopyTablesOptions,
	jobs <-chan copyJob,
	results chan<- copyWorkerResult,
) {
	for job := range jobs {
		result, err := copyTableWithFactories(ctx, opts, job.table)
		results <- copyWorkerResult{index: job.index, result: result, err: err}
		if err != nil {
			cancel()
			return
		}
	}
}

func copyTableWithFactories(ctx context.Context, opts CopyTablesOptions, table models.Table) (models.SyncResult, error) {
	source, err := opts.SourceFactory(ctx)
	if err != nil {
		return models.SyncResult{}, fmt.Errorf("open source connection for %s: %w", tableEventName(table), err)
	}
	target, err := opts.TargetFactory(ctx)
	if err != nil {
		return models.SyncResult{}, errors.Join(
			fmt.Errorf("open target connection for %s: %w", tableEventName(table), err),
			source.Close(context.WithoutCancel(ctx)),
		)
	}

	result, copyErr := CopyTable(ctx, CopyTableOptions{
		Table:      table,
		SnapshotID: opts.SnapshotID,
		Source:     source,
		Target:     target,
		Observer:   opts.Observer,
		Clock:      opts.Clock,
	})
	closeErr := target.Close(context.WithoutCancel(ctx))
	return result, errors.Join(copyErr, closeErr)
}

func aggregateCopyResults(
	started time.Time,
	finished time.Time,
	results []models.SyncResult,
	err error,
) *models.SyncResult {
	aggregate := &models.SyncResult{
		StartedAt:  started,
		FinishedAt: finished,
		Stages:     copyStages(started, finished),
		Err:        err,
	}
	for _, result := range results {
		aggregate.BytesCopied += result.BytesCopied
		aggregate.RowsCopied += result.RowsCopied
		aggregate.TablesCopied += result.TablesCopied
	}
	return aggregate
}

func copyTableFailed(started time.Time, finished time.Time, bytesCopied int64, rowsCopied int64, err error) models.SyncResult {
	return models.SyncResult{
		StartedAt:   started,
		FinishedAt:  finished,
		BytesCopied: bytesCopied,
		RowsCopied:  rowsCopied,
		Stages:      copyStages(started, finished),
		Err:         err,
	}
}

func copyStages(started time.Time, finished time.Time) map[string]time.Duration {
	return map[string]time.Duration{copyStage: finished.Sub(started)}
}

func emitCopyEvent(
	ctx context.Context,
	observer engine.ProgressObserver,
	name string,
	table models.Table,
	bytesCopied int64,
	rowsCopied int64,
	started time.Time,
	now time.Time,
) {
	if observer == nil {
		return
	}
	observer.OnEvent(ctx, engine.Event{
		Time:        now,
		Level:       "info",
		Name:        name,
		Stage:       copyStage,
		Engine:      string(engine.ModeNative),
		Table:       tableEventName(table),
		Rows:        rowsCopied,
		Estimated:   table.SizeBytes,
		Bytes:       bytesCopied,
		Percent:     progressPercent(bytesCopied, table.SizeBytes),
		BytesPerSec: progressBytesPerSecond(bytesCopied, now.Sub(started)),
		Duration:    now.Sub(started),
	})
}

func copyToSQL(quotedTable string) string {
	return "COPY " + quotedTable + " TO STDOUT WITH (FORMAT binary)"
}

func copyFromSQL(quotedTable string) string {
	return "COPY " + quotedTable + " FROM STDIN WITH (FORMAT binary)"
}

func rowsAffected(tag pgconn.CommandTag) int64 {
	return tag.RowsAffected()
}

func workerCount(threads int) int {
	if threads <= 0 {
		return 1
	}
	return threads
}
