package native

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/pgdb"
)

func TestCopyTableUsesQuotedSQLAppliesSnapshotAndEmitsEvents(t *testing.T) {
	t.Parallel()
	source := &copyFakeConn{copyToData: []byte("hello"), rows: 2}
	target := &copyFakeConn{rows: 2}
	observer := &recordingObserver{}
	clock := newManualClock()
	table := models.Table{Schema: `we"ird`, Name: "users", SizeBytes: 5}

	result, err := runCopyTable(t, CopyTableOptions{
		Table:      table,
		SnapshotID: "00000003-0000001B-1",
		Source:     source,
		Target:     target,
		Observer:   observer,
		Clock:      clock,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(5), result.BytesCopied)
	assert.Equal(t, int64(2), result.RowsCopied)
	assert.Equal(t, 1, result.TablesCopied)
	assert.Equal(t, []string{
		"exec:" + beginRepeatableReadSQL,
		"exec:SET TRANSACTION SNAPSHOT '00000003-0000001B-1'",
		`copyTo:COPY "we""ird"."users" TO STDOUT WITH (FORMAT binary)`,
		"exec:" + rollbackSQL,
		"close",
	}, source.Operations())
	assert.Equal(t, []string{`COPY "we""ird"."users" FROM STDIN WITH (FORMAT binary)`}, target.CopyFromSQLs())
	assert.Equal(t, []byte("hello"), target.Received())

	events := observer.Events()
	assert.Contains(t, eventNames(events), engine.EventTableCopyStart)
	assert.Contains(t, eventNames(events), engine.EventTableCopyProgress)
	assert.Contains(t, eventNames(events), engine.EventTableCopyDone)
	assert.True(t, hasProgressBytes(events, 5))
}

func TestCopyTableTargetCopyErrorCancelsSourceAndReturnsTargetError(t *testing.T) {
	t.Parallel()
	targetErr := errors.New("target copy failed")
	copyToStarted := make(chan struct{})
	source := &copyFakeConn{
		copyToData:    []byte("unread data blocks until target closes"),
		copyToStarted: copyToStarted,
	}
	target := &copyFakeConn{
		copyFromErr: targetErr,
		beforeCopyFrom: func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-copyToStarted:
				return nil
			}
		},
	}

	result, err := runCopyTable(t, CopyTableOptions{
		Table:      models.Table{Schema: "public", Name: "users"},
		SnapshotID: "snapshot-id",
		Source:     source,
		Target:     target,
		Clock:      newManualClock(),
	})

	require.ErrorIs(t, err, targetErr)
	assert.ErrorIs(t, result.Err, targetErr)
	assert.Contains(t, source.Operations(), "close")
}

func TestCopyTableSourceCopyErrorClosesPipeAndReturnsSourceError(t *testing.T) {
	t.Parallel()
	sourceErr := errors.New("source copy failed")
	source := &copyFakeConn{copyToErr: sourceErr}
	target := &copyFakeConn{}

	result, err := runCopyTable(t, CopyTableOptions{
		Table:      models.Table{Schema: "public", Name: "users"},
		SnapshotID: "snapshot-id",
		Source:     source,
		Target:     target,
		Clock:      newManualClock(),
	})

	require.ErrorIs(t, err, sourceErr)
	assert.ErrorIs(t, result.Err, sourceErr)
	assert.Equal(t, int64(0), result.BytesCopied)
}

func TestCopyTableReturnsSourceCleanupErrorAfterTargetSuccess(t *testing.T) {
	t.Parallel()
	closeErr := errors.New("source close failed")
	source := &copyFakeConn{copyToData: []byte("ok"), closeErr: closeErr}
	target := &copyFakeConn{rows: 1}

	result, err := runCopyTable(t, CopyTableOptions{
		Table:      models.Table{Schema: "public", Name: "users", SizeBytes: 2},
		SnapshotID: "snapshot-id",
		Source:     source,
		Target:     target,
	})

	require.ErrorIs(t, err, closeErr)
	assert.ErrorIs(t, result.Err, closeErr)
	assert.Equal(t, int64(2), result.BytesCopied)
	assert.Equal(t, int64(1), result.RowsCopied)
}

func TestCopyTableValidatesOptionsAndTableName(t *testing.T) {
	t.Parallel()
	_, err := CopyTable(context.Background(), CopyTableOptions{Target: &copyFakeConn{}})
	require.EqualError(t, err, "source connection is required")

	_, err = CopyTable(context.Background(), CopyTableOptions{Source: &copyFakeConn{}})
	require.EqualError(t, err, "target connection is required")

	_, err = CopyTable(context.Background(), CopyTableOptions{Source: &copyFakeConn{}, Target: &copyFakeConn{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quote schema")
}

func TestCopyTablesRunsNoMoreThanThreadsAndAggregatesResults(t *testing.T) {
	t.Parallel()
	tables := []models.Table{
		{Schema: "public", Name: "a", SizeBytes: 1},
		{Schema: "public", Name: "b", SizeBytes: 1},
		{Schema: "public", Name: "c", SizeBytes: 1},
		{Schema: "public", Name: "d", SizeBytes: 1},
	}
	gate := make(chan struct{})
	counter := &concurrencyCounter{}
	factory := &copyConnFactory{
		sourceBuilder: func() *copyFakeConn {
			return &copyFakeConn{
				copyToData: []byte("x"),
				beforeCopyTo: func(ctx context.Context) error {
					counter.Enter()
					select {
					case <-ctx.Done():
						counter.Leave()
						return ctx.Err()
					case <-gate:
						counter.Leave()
						return nil
					}
				},
			}
		},
		targetBuilder: func() *copyFakeConn { return &copyFakeConn{rows: 1} },
	}
	done := make(chan copyTablesCall, 1)
	go func() {
		result, err := CopyTables(context.Background(), CopyTablesOptions{
			Tables:        tables,
			SnapshotID:    "snapshot-id",
			Threads:       2,
			SourceFactory: factory.Source,
			TargetFactory: factory.Target,
			Clock:         newManualClock(),
		})
		done <- copyTablesCall{result: result, err: err}
	}()

	counter.WaitForActive(t, 2)
	assert.Equal(t, 2, counter.Max())
	close(gate)

	call := waitCopyTables(t, done)
	require.NoError(t, call.err)
	require.NotNil(t, call.result)
	assert.Equal(t, int64(4), call.result.BytesCopied)
	assert.Equal(t, int64(4), call.result.RowsCopied)
	assert.Equal(t, 4, call.result.TablesCopied)
	assert.Equal(t, 2, counter.Max())
}

func TestCopyTablesCancelsRemainingWorkOnFirstError(t *testing.T) {
	t.Parallel()
	targetErr := errors.New("target failed")
	factory := &copyConnFactory{
		sourceBuilder: func() *copyFakeConn { return &copyFakeConn{copyToData: []byte("x")} },
		targetBuilder: func() *copyFakeConn { return &copyFakeConn{copyFromErr: targetErr} },
	}

	result, err := CopyTables(context.Background(), CopyTablesOptions{
		Tables: []models.Table{
			{Schema: "public", Name: "first", SizeBytes: 1},
			{Schema: "public", Name: "second", SizeBytes: 1},
		},
		SnapshotID:    "snapshot-id",
		Threads:       1,
		SourceFactory: factory.Source,
		TargetFactory: factory.Target,
		Clock:         newManualClock(),
	})

	require.ErrorIs(t, err, targetErr)
	require.NotNil(t, result)
	assert.ErrorIs(t, result.Err, targetErr)
	assert.Equal(t, 1, factory.SourceCalls())
}

func TestCopyTablesHandlesEmptyAndDefaultThreadInputs(t *testing.T) {
	t.Parallel()
	factory := &copyConnFactory{
		sourceBuilder: func() *copyFakeConn { return &copyFakeConn{copyToData: []byte("z")} },
		targetBuilder: func() *copyFakeConn { return &copyFakeConn{rows: 1} },
	}
	emptyResult, err := CopyTables(context.Background(), CopyTablesOptions{Clock: newManualClock()})
	require.NoError(t, err)
	require.NotNil(t, emptyResult)
	assert.Zero(t, emptyResult.TablesCopied)
	assert.Zero(t, factory.SourceCalls())

	result, err := CopyTables(context.Background(), CopyTablesOptions{
		Tables:        []models.Table{{Schema: "public", Name: "one", SizeBytes: 1}},
		SnapshotID:    "snapshot-id",
		Threads:       0,
		SourceFactory: factory.Source,
		TargetFactory: factory.Target,
		Clock:         newManualClock(),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.TablesCopied)
	assert.Equal(t, 1, workerCount(0))
}

func TestCopyTablesValidatesFactoriesAndClosesSourceOnTargetFactoryError(t *testing.T) {
	t.Parallel()
	table := models.Table{Schema: "public", Name: "users"}
	result, err := CopyTables(context.Background(), CopyTablesOptions{Tables: []models.Table{table}})
	require.EqualError(t, err, "source factory is required")
	assert.Nil(t, result)

	result, err = CopyTables(context.Background(), CopyTablesOptions{
		Tables:        []models.Table{table},
		SourceFactory: func(context.Context) (pgdb.CopyConn, error) { return &copyFakeConn{}, nil },
	})
	require.EqualError(t, err, "target factory is required")
	assert.Nil(t, result)

	sourceErr := errors.New("source dial failed")
	factory := &copyConnFactory{sourceErr: sourceErr}
	result, err = CopyTables(context.Background(), CopyTablesOptions{
		Tables:        []models.Table{table},
		SourceFactory: factory.Source,
		TargetFactory: factory.Target,
	})
	require.ErrorIs(t, err, sourceErr)
	require.NotNil(t, result)
	assert.ErrorIs(t, result.Err, sourceErr)

	targetErr := errors.New("target dial failed")
	closeErr := errors.New("source close failed")
	source := &copyFakeConn{closeErr: closeErr}
	factory = &copyConnFactory{
		sourceBuilder: func() *copyFakeConn { return source },
		targetErr:     targetErr,
	}
	result, err = CopyTables(context.Background(), CopyTablesOptions{
		Tables:        []models.Table{table},
		SourceFactory: factory.Source,
		TargetFactory: factory.Target,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, targetErr)
	assert.ErrorIs(t, err, closeErr)
	assert.ErrorIs(t, result.Err, targetErr)
	assert.True(t, source.Closed())
}

func TestCopyTablesReturnsTargetCloseError(t *testing.T) {
	t.Parallel()
	closeErr := errors.New("target close failed")
	factory := &copyConnFactory{
		sourceBuilder: func() *copyFakeConn { return &copyFakeConn{copyToData: []byte("x")} },
		targetBuilder: func() *copyFakeConn { return &copyFakeConn{rows: 1, closeErr: closeErr} },
	}

	result, err := CopyTables(context.Background(), CopyTablesOptions{
		Tables:        []models.Table{{Schema: "public", Name: "users", SizeBytes: 1}},
		SnapshotID:    "snapshot-id",
		SourceFactory: factory.Source,
		TargetFactory: factory.Target,
		Clock:         newManualClock(),
	})

	require.ErrorIs(t, err, closeErr)
	require.NotNil(t, result)
	assert.ErrorIs(t, result.Err, closeErr)
	assert.Equal(t, 1, result.TablesCopied)
}

type copyTableCall struct {
	result models.SyncResult
	err    error
}

func runCopyTable(t *testing.T, opts CopyTableOptions) (models.SyncResult, error) {
	t.Helper()
	done := make(chan copyTableCall, 1)
	go func() {
		result, err := CopyTable(context.Background(), opts)
		done <- copyTableCall{result: result, err: err}
	}()
	select {
	case call := <-done:
		return call.result, call.err
	case <-time.After(2 * time.Second):
		t.Fatal("CopyTable timed out")
		return models.SyncResult{}, nil
	}
}

type copyTablesCall struct {
	result *models.SyncResult
	err    error
}

func waitCopyTables(t *testing.T, done <-chan copyTablesCall) copyTablesCall {
	t.Helper()
	select {
	case call := <-done:
		return call
	case <-time.After(2 * time.Second):
		t.Fatal("CopyTables timed out")
		return copyTablesCall{}
	}
}

type copyFakeConn struct {
	mu sync.Mutex

	execCalls      []copyExecCall
	operations     []string
	copyToSQLs     []string
	copyFromSQLs   []string
	received       []byte
	copyToData     []byte
	copyToErr      error
	copyFromErr    error
	closeErr       error
	rows           int64
	closed         bool
	copyToStarted  chan struct{}
	beforeCopyTo   func(context.Context) error
	beforeCopyFrom func(context.Context) error
}

type copyExecCall struct {
	sql  string
	args []any
}

func (c *copyFakeConn) Query(ctx context.Context, _ string, _ ...any) (pgdb.Rows, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("unexpected query")
}

func (c *copyFakeConn) QueryRow(ctx context.Context, _ string, _ ...any) pgdb.Row {
	if err := ctx.Err(); err != nil {
		return &fakeRow{err: err}
	}
	return &fakeRow{err: errors.New("unexpected query row")}
}

func (c *copyFakeConn) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	c.mu.Lock()
	c.execCalls = append(c.execCalls, copyExecCall{sql: sql, args: append([]any(nil), args...)})
	c.operations = append(c.operations, "exec:"+sql)
	c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return pgconn.CommandTag{}, err
	}
	return pgconn.CommandTag{}, nil
}

func (c *copyFakeConn) Close(ctx context.Context) error {
	c.mu.Lock()
	c.closed = true
	c.operations = append(c.operations, "close")
	closeErr := c.closeErr
	c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return closeErr
}

func (c *copyFakeConn) CopyTo(ctx context.Context, w io.Writer, sql string) (pgconn.CommandTag, error) {
	c.mu.Lock()
	c.copyToSQLs = append(c.copyToSQLs, sql)
	c.operations = append(c.operations, "copyTo:"+sql)
	started := c.copyToStarted
	before := c.beforeCopyTo
	data := append([]byte(nil), c.copyToData...)
	copyErr := c.copyToErr
	rows := c.rows
	c.mu.Unlock()
	closeOnce(started)
	if before != nil {
		if err := before(ctx); err != nil {
			return pgconn.CommandTag{}, err
		}
	}
	if copyErr != nil {
		return pgconn.CommandTag{}, copyErr
	}
	if len(data) > 0 {
		_, err := w.Write(data)
		if err != nil {
			return pgconn.CommandTag{}, err
		}
	}
	return pgconn.NewCommandTag(fmt.Sprintf("COPY %d", rows)), nil
}

func (c *copyFakeConn) CopyFrom(ctx context.Context, r io.Reader, sql string) (pgconn.CommandTag, error) {
	c.mu.Lock()
	c.copyFromSQLs = append(c.copyFromSQLs, sql)
	before := c.beforeCopyFrom
	copyErr := c.copyFromErr
	rows := c.rows
	c.mu.Unlock()
	if before != nil {
		if err := before(ctx); err != nil {
			return pgconn.CommandTag{}, err
		}
	}
	if copyErr != nil {
		return pgconn.CommandTag{}, copyErr
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	c.mu.Lock()
	c.received = append(c.received, data...)
	c.mu.Unlock()
	return pgconn.NewCommandTag(fmt.Sprintf("COPY %d", rows)), nil
}

func (c *copyFakeConn) ExecMulti(ctx context.Context, _ string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.New("unexpected exec multi")
}

func (c *copyFakeConn) Operations() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.operations...)
}

func (c *copyFakeConn) CopyFromSQLs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.copyFromSQLs...)
}

func (c *copyFakeConn) Received() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.received...)
}

func (c *copyFakeConn) Closed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func closeOnce(ch chan struct{}) {
	if ch == nil {
		return
	}
	select {
	case <-ch:
	default:
		close(ch)
	}
}

type copyConnFactory struct {
	mu sync.Mutex

	sourceBuilder func() *copyFakeConn
	targetBuilder func() *copyFakeConn
	sourceErr     error
	targetErr     error
	sourceCalls   int
	targetCalls   int
}

func (f *copyConnFactory) Source(ctx context.Context) (pgdb.CopyConn, error) {
	f.mu.Lock()
	f.sourceCalls++
	builder := f.sourceBuilder
	err := f.sourceErr
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if builder == nil {
		return &copyFakeConn{copyToData: []byte("x")}, nil
	}
	return builder(), nil
}

func (f *copyConnFactory) Target(ctx context.Context) (pgdb.CopyConn, error) {
	f.mu.Lock()
	f.targetCalls++
	builder := f.targetBuilder
	err := f.targetErr
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if builder == nil {
		return &copyFakeConn{}, nil
	}
	return builder(), nil
}

func (f *copyConnFactory) SourceCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sourceCalls
}

type concurrencyCounter struct {
	mu      sync.Mutex
	active  int
	max     int
	changed chan struct{}
}

func (c *concurrencyCounter) Enter() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.changed == nil {
		c.changed = make(chan struct{})
	}
	c.active++
	if c.active > c.max {
		c.max = c.active
	}
	close(c.changed)
	c.changed = make(chan struct{})
}

func (c *concurrencyCounter) Leave() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active--
	close(c.changed)
	c.changed = make(chan struct{})
}

func (c *concurrencyCounter) Max() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.max
}

func (c *concurrencyCounter) WaitForActive(t *testing.T, want int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		c.mu.Lock()
		if c.active == want {
			c.mu.Unlock()
			return
		}
		changed := c.changed
		if changed == nil {
			changed = make(chan struct{})
			c.changed = changed
		}
		c.mu.Unlock()
		select {
		case <-changed:
		case <-deadline:
			t.Fatalf("timed out waiting for %d active copies", want)
		}
	}
}

func eventNames(events []engine.Event) []string {
	names := make([]string, 0, len(events))
	for _, event := range events {
		names = append(names, event.Name)
	}
	return names
}

func hasProgressBytes(events []engine.Event, bytes int64) bool {
	for _, event := range events {
		if event.Name == engine.EventTableCopyProgress && event.Bytes == bytes {
			return true
		}
	}
	return false
}
