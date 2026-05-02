package pgdb

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/config"
)

func TestConnectorConstructors(t *testing.T) {
	t.Parallel()

	assert.NotNil(t, NewConnector())
	assert.NotNil(t, NewPgxConnector())
}

func TestPgxConnectorConnectRedactsDSNInErrors(t *testing.T) {
	t.Parallel()
	var capturedConnString string
	connector := &PgxConnector{open: func(ctx context.Context, connString string) (CopyConn, error) {
		capturedConnString = connString
		return nil, errors.New("dial failed")
	}}
	ep := Endpoint{
		Host:     "prod.example.com",
		Port:     5432,
		User:     "alice",
		Password: "super-secret",
		Database: "app",
		SSLMode:  "require",
	}

	_, err := connector.Connect(context.Background(), ep)

	require.Error(t, err)
	assert.Contains(t, capturedConnString, "super-secret")
	assert.Contains(t, err.Error(), "xxxxx")
	assert.NotContains(t, err.Error(), "super-secret")
}

func TestPgxConnectorConnectReturnsBuildErrors(t *testing.T) {
	t.Parallel()
	connector := &PgxConnector{open: func(ctx context.Context, connString string) (CopyConn, error) {
		require.Fail(t, "opener must not be called for invalid endpoints")
		return nil, nil
	}}

	_, err := connector.Connect(context.Background(), Endpoint{Port: 5432})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "build pg connection string")
}

func TestPgxConnectorConnectReturnsOpenedConnection(t *testing.T) {
	t.Parallel()
	want := &fakeCopyConn{}
	connector := &PgxConnector{open: func(ctx context.Context, connString string) (CopyConn, error) {
		return want, nil
	}}
	ep := Endpoint{Host: "localhost", Port: 5432, Database: "app"}

	got, err := connector.Connect(context.Background(), ep)

	require.NoError(t, err)
	assert.Same(t, want, got)
}

func TestPgxConnectorZeroValueUsesProductionOpener(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&PgxConnector{}).Connect(ctx, Endpoint{Host: "localhost", Port: 5432, Database: "app"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "connect postgres")
}

func TestConfigConnectorPassesExactDatabaseOverride(t *testing.T) {
	t.Parallel()
	wrapped := &fakeEndpointConnector{conn: &fakeCopyConn{}}
	connector := NewConfigConnector(wrapped)
	cfg := config.Connection{
		Host:     "prod.example.com",
		Port:     5432,
		User:     "alice",
		Database: "configured",
		SSLMode:  "require",
	}

	_, err := connector.Connect(context.Background(), cfg, " cli-db ")

	require.NoError(t, err)
	require.Len(t, wrapped.endpoints, 1)
	assert.Equal(t, " cli-db ", wrapped.endpoints[0].Database)
}

func TestPgxConnCloseIsIdempotent(t *testing.T) {
	t.Parallel()
	closeCalls := 0
	conn := newPgxConnWithOperations(connOperations{
		close: func(ctx context.Context) error {
			closeCalls++
			return nil
		},
	})

	require.NoError(t, conn.Close(context.Background()))
	require.NoError(t, conn.Close(context.Background()))

	assert.Equal(t, 1, closeCalls)
}

func TestPgxConnDelegatesOperations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rows := &fakeRows{}
	row := &fakeRow{}
	var queries []string
	var queryArgs []any
	var execSQL string
	var copyToSQL string
	var copyFromSQL string
	var copyFromBody string
	var execMultiSQL string
	conn := newPgxConnWithOperations(connOperations{
		query: func(ctx context.Context, sql string, args ...any) (Rows, error) {
			queries = append(queries, sql)
			queryArgs = args
			return rows, nil
		},
		queryRow: func(ctx context.Context, sql string, args ...any) Row {
			queries = append(queries, sql)
			queryArgs = args
			return row
		},
		exec: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			execSQL = sql
			queryArgs = args
			return pgconn.CommandTag{}, nil
		},
		copyTo: func(ctx context.Context, w io.Writer, sql string) (pgconn.CommandTag, error) {
			copyToSQL = sql
			_, err := w.Write([]byte("copy-bytes"))
			return pgconn.CommandTag{}, err
		},
		copyFrom: func(ctx context.Context, r io.Reader, sql string) (pgconn.CommandTag, error) {
			copyFromSQL = sql
			body, err := io.ReadAll(r)
			require.NoError(t, err)
			copyFromBody = string(body)
			return pgconn.CommandTag{}, nil
		},
		execMulti: func(ctx context.Context, sql string) error {
			execMultiSQL = sql
			return nil
		},
	})

	gotRows, err := conn.Query(ctx, "select rows", 1)
	require.NoError(t, err)
	gotRow := conn.QueryRow(ctx, "select row", 2)
	_, err = conn.Exec(ctx, "delete", 3)
	require.NoError(t, err)
	copyToBuffer := &bytes.Buffer{}
	_, err = conn.CopyTo(ctx, copyToBuffer, "copy to")
	require.NoError(t, err)
	_, err = conn.CopyFrom(ctx, bytes.NewBufferString("copy input"), "copy from")
	require.NoError(t, err)
	require.NoError(t, conn.ExecMulti(ctx, "select 1; select 2"))

	assert.Same(t, rows, gotRows)
	assert.Same(t, row, gotRow)
	assert.Equal(t, []string{"select rows", "select row"}, queries)
	assert.Equal(t, []any{3}, queryArgs)
	assert.Equal(t, "delete", execSQL)
	assert.Equal(t, "copy to", copyToSQL)
	assert.Equal(t, "copy-bytes", copyToBuffer.String())
	assert.Equal(t, "copy from", copyFromSQL)
	assert.Equal(t, "copy input", copyFromBody)
	assert.Equal(t, "select 1; select 2", execMultiSQL)
}

func TestPgxConnNativeMethodsDelegateToPgx(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	require.Panics(t, func() { _, _ = NewPgxConn(nil).Query(ctx, "select 1") })
	require.Panics(t, func() { _ = NewPgxConn(nil).QueryRow(ctx, "select 1") })
	require.Panics(t, func() { _, _ = NewPgxConn(nil).Exec(ctx, "select 1") })
	require.Panics(t, func() { _ = NewPgxConn(nil).Close(ctx) })
	require.Panics(t, func() { _, _ = NewPgxConn(nil).CopyTo(ctx, &bytes.Buffer{}, "copy t to stdout") })
	require.Panics(t, func() { _, _ = NewPgxConn(nil).CopyFrom(ctx, bytes.NewBuffer(nil), "copy t from stdin") })
	require.Panics(t, func() { _ = NewPgxConn(nil).ExecMulti(ctx, "select 1") })
}

func TestOpenPgxConnUsesPgxConnect(t *testing.T) {
	t.Parallel()

	_, err := openPgxConn(context.Background(), "postgres://%")

	require.Error(t, err)
}

func TestOpenPgxConnWithReturnsConnectionAndErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	expectedErr := errors.New("connect failed")

	conn, err := openPgxConnWith(ctx, "postgres://localhost/app", func(ctx context.Context, connString string) (*pgx.Conn, error) {
		return nil, nil
	})
	require.NoError(t, err)
	assert.IsType(t, &PgxConn{}, conn)

	conn, err = openPgxConnWith(ctx, "postgres://localhost/app", func(ctx context.Context, connString string) (*pgx.Conn, error) {
		return nil, expectedErr
	})
	require.ErrorIs(t, err, expectedErr)
	assert.Nil(t, conn)
}

func TestReadAllResultsReturnsReaderError(t *testing.T) {
	t.Parallel()
	expectedErr := errors.New("read failed")
	reader := &fakeMultiResultReader{err: expectedErr}

	err := readAllResults(reader)

	require.ErrorIs(t, err, expectedErr)
	assert.Equal(t, 1, reader.calls)
}

type fakeEndpointConnector struct {
	conn      CopyConn
	endpoints []Endpoint
}

func (c *fakeEndpointConnector) Connect(ctx context.Context, ep Endpoint) (CopyConn, error) {
	c.endpoints = append(c.endpoints, ep)
	return c.conn, nil
}

type fakeCopyConn struct{}

func (c *fakeCopyConn) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	return &fakeRows{}, nil
}

func (c *fakeCopyConn) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return &fakeRow{}
}

func (c *fakeCopyConn) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (c *fakeCopyConn) Close(ctx context.Context) error {
	return nil
}

func (c *fakeCopyConn) CopyTo(ctx context.Context, w io.Writer, sql string) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (c *fakeCopyConn) CopyFrom(ctx context.Context, r io.Reader, sql string) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (c *fakeCopyConn) ExecMulti(ctx context.Context, sql string) error {
	return nil
}

type fakeRows struct{}

func (r *fakeRows) Close() {}

func (r *fakeRows) Err() error {
	return nil
}

func (r *fakeRows) Next() bool {
	return false
}

func (r *fakeRows) Scan(dest ...any) error {
	return nil
}

type fakeRow struct{}

func (r *fakeRow) Scan(dest ...any) error {
	return nil
}

type fakeMultiResultReader struct {
	err   error
	calls int
}

func (r *fakeMultiResultReader) ReadAll() ([]*pgconn.Result, error) {
	r.calls++
	return nil, r.err
}
