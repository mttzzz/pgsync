package native

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/pgdb"
)

func TestExportSnapshotStartsRepeatableReadAndKeepsConnectionOpen(t *testing.T) {
	t.Parallel()
	conn := &fakeCopyConn{queryRow: &fakeRow{value: "00000003-0000001B-1"}}

	snapshot, err := ExportSnapshot(context.Background(), conn)

	require.NoError(t, err)
	require.NotNil(t, snapshot)
	assert.Equal(t, "00000003-0000001B-1", snapshot.ID)
	assert.Same(t, conn, snapshot.Conn)
	assert.Equal(t, []fakeExecCall{{sql: beginRepeatableReadSQL}}, conn.execCalls)
	assert.Equal(t, []string{exportSnapshotSQL}, conn.queryRowSQL)
	assert.False(t, conn.closed)

	require.NoError(t, snapshot.Close(context.Background()))
	assert.Equal(t, []fakeExecCall{{sql: beginRepeatableReadSQL}, {sql: rollbackSQL}}, conn.execCalls)
	assert.True(t, conn.closed)
}

func TestExportSnapshotBeginErrorPropagatesContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conn := &fakeCopyConn{queryRow: &fakeRow{value: "unused"}}

	snapshot, err := ExportSnapshot(ctx, conn)

	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, snapshot)
	assert.Equal(t, []fakeExecCall{{sql: beginRepeatableReadSQL}}, conn.execCalls)
	assert.Empty(t, conn.queryRowSQL)
	assert.False(t, conn.closed)
}

func TestExportSnapshotFailureAfterBeginRollsBackAndClosesConnection(t *testing.T) {
	t.Parallel()
	exportErr := errors.New("export failed")
	rollbackErr := errors.New("rollback failed")
	closeErr := errors.New("close failed")
	conn := &fakeCopyConn{
		execErrs: []error{nil, rollbackErr},
		queryRow: &fakeRow{err: exportErr},
		closeErr: closeErr,
	}

	snapshot, err := ExportSnapshot(context.Background(), conn)

	require.Error(t, err)
	assert.ErrorIs(t, err, exportErr)
	assert.ErrorIs(t, err, rollbackErr)
	assert.ErrorIs(t, err, closeErr)
	assert.Nil(t, snapshot)
	assert.Equal(t, []fakeExecCall{{sql: beginRepeatableReadSQL}, {sql: rollbackSQL}}, conn.execCalls)
	assert.True(t, conn.closed)
}

func TestApplySnapshotStartsRepeatableReadAndSetsSnapshotLiteral(t *testing.T) {
	t.Parallel()
	conn := &fakeCopyConn{}

	err := ApplySnapshot(context.Background(), conn, "00000003-'quoted'-1")

	require.NoError(t, err)
	assert.Equal(t, []fakeExecCall{
		{sql: beginRepeatableReadSQL},
		{sql: "SET TRANSACTION SNAPSHOT '00000003-''quoted''-1'"},
	}, conn.execCalls)
	assert.False(t, conn.closed)
}

func TestApplySnapshotBeginErrorPropagatesContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conn := &fakeCopyConn{}

	err := ApplySnapshot(ctx, conn, "00000003-0000001B-1")

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []fakeExecCall{{sql: beginRepeatableReadSQL}}, conn.execCalls)
	assert.False(t, conn.closed)
}

func TestApplySnapshotSetFailureRollsBackWorkerTransaction(t *testing.T) {
	t.Parallel()
	setErr := errors.New("set failed")
	rollbackErr := errors.New("rollback failed")
	conn := &fakeCopyConn{execErrs: []error{nil, setErr, rollbackErr}}

	err := ApplySnapshot(context.Background(), conn, "00000003-0000001B-1")

	require.Error(t, err)
	assert.ErrorIs(t, err, setErr)
	assert.ErrorIs(t, err, rollbackErr)
	assert.Equal(t, []fakeExecCall{
		{sql: beginRepeatableReadSQL},
		{sql: "SET TRANSACTION SNAPSHOT '00000003-0000001B-1'"},
		{sql: rollbackSQL},
	}, conn.execCalls)
	assert.False(t, conn.closed)
}

func TestSnapshotCloseJoinsRollbackAndCloseErrors(t *testing.T) {
	t.Parallel()
	rollbackErr := errors.New("rollback failed")
	closeErr := errors.New("close failed")
	conn := &fakeCopyConn{execErrs: []error{rollbackErr}, closeErr: closeErr}
	snapshot := &Snapshot{ID: "00000003-0000001B-1", Conn: conn}

	err := snapshot.Close(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, rollbackErr)
	assert.ErrorIs(t, err, closeErr)
	assert.Equal(t, []fakeExecCall{{sql: rollbackSQL}}, conn.execCalls)
	assert.True(t, conn.closed)
}

type fakeExecCall struct {
	sql  string
	args []any
}

type fakeCopyConn struct {
	execCalls   []fakeExecCall
	execErrs    []error
	queryRowSQL []string
	queryRow    pgdb.Row
	closeErr    error
	closed      bool
}

func (c *fakeCopyConn) Query(ctx context.Context, sql string, args ...any) (pgdb.Rows, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("unexpected query")
}

func (c *fakeCopyConn) QueryRow(ctx context.Context, sql string, args ...any) pgdb.Row {
	c.queryRowSQL = append(c.queryRowSQL, sql)
	if err := ctx.Err(); err != nil {
		return &fakeRow{err: err}
	}
	return c.queryRow
}

func (c *fakeCopyConn) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	c.execCalls = append(c.execCalls, fakeExecCall{sql: sql, args: args})
	if err := ctx.Err(); err != nil {
		return pgconn.CommandTag{}, err
	}
	if len(c.execErrs) == 0 {
		return pgconn.CommandTag{}, nil
	}
	err := c.execErrs[0]
	c.execErrs = c.execErrs[1:]
	return pgconn.CommandTag{}, err
}

func (c *fakeCopyConn) Close(ctx context.Context) error {
	c.closed = true
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.closeErr
}

func (c *fakeCopyConn) CopyTo(ctx context.Context, w io.Writer, sql string) (pgconn.CommandTag, error) {
	if err := ctx.Err(); err != nil {
		return pgconn.CommandTag{}, err
	}
	return pgconn.CommandTag{}, errors.New("unexpected copy to")
}

func (c *fakeCopyConn) CopyFrom(ctx context.Context, r io.Reader, sql string) (pgconn.CommandTag, error) {
	if err := ctx.Err(); err != nil {
		return pgconn.CommandTag{}, err
	}
	return pgconn.CommandTag{}, errors.New("unexpected copy from")
}

func (c *fakeCopyConn) ExecMulti(ctx context.Context, sql string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.New("unexpected exec multi")
}

type fakeRow struct {
	value string
	err   error
}

func (r *fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*(dest[0].(*string)) = r.value
	return nil
}
