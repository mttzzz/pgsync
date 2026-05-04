package native

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/pgdb"
)

func TestTargetManagerResetDatabaseDropsAndCreatesQuotedDatabase(t *testing.T) {
	t.Parallel()
	conn := &targetFakeConn{}
	connector := &targetFakeConnector{conn: conn}
	manager := &TargetManager{Connector: connector}
	local := targetLocalConnection("")

	err := manager.ResetDatabase(context.Background(), local, ` app"prod `)

	require.NoError(t, err)
	require.Len(t, connector.calls, 1)
	assert.Equal(t, pgdb.Endpoint{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "localpass",
		Database: "postgres",
		SSLMode:  "disable",
	}, connector.calls[0].endpoint)
	assert.Equal(t, []targetExecCall{
		{sql: terminateTargetSessionsSQL, args: []any{`app"prod`}},
		{sql: `DROP DATABASE IF EXISTS "app""prod"`},
		{sql: `CREATE DATABASE "app""prod"`},
	}, conn.execCalls)
	assert.True(t, conn.closed)
}

func TestTargetManagerResetDatabaseUsesConfiguredMaintenanceDatabaseAndRedactsConnectError(t *testing.T) {
	t.Parallel()
	connectErr := errors.New("dial failed with localpass and password=localpass")
	connector := &targetFakeConnector{err: connectErr}
	manager := &TargetManager{Connector: connector}
	local := targetLocalConnection("maintenance")

	err := manager.ResetDatabase(context.Background(), local, "appdb")

	require.Error(t, err)
	assert.NotErrorIs(t, err, connectErr)
	assert.NotContains(t, err.Error(), "localpass")
	assert.Contains(t, err.Error(), "xxxxx")
	assert.Contains(t, err.Error(), "maintenance")
	require.Len(t, connector.calls, 1)
	assert.Equal(t, "maintenance", connector.calls[0].endpoint.Database)
}

func TestTargetManagerResetDatabaseFallsBackToPostgresWhenLocalDatabaseEqualsTarget(t *testing.T) {
	t.Parallel()
	conn := &targetFakeConn{}
	connector := &targetFakeConnector{conn: conn}
	manager := &TargetManager{Connector: connector}
	/* POSTGRES_URL=postgres://...@localhost/appdb fills both local.Database and the
	 * sync target with "appdb"; connecting maintenance to "appdb" would self-block
	 * the subsequent DROP DATABASE with SQLSTATE 55006. */
	local := targetLocalConnection("appdb")

	err := manager.ResetDatabase(context.Background(), local, "appdb")

	require.NoError(t, err)
	require.Len(t, connector.calls, 1)
	assert.Equal(t, "postgres", connector.calls[0].endpoint.Database)
}

func TestTargetManagerResetDatabaseRequiresManagerAndConnector(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		manager *TargetManager
		want    string
	}{
		{name: "manager", manager: nil, want: "target manager is required"},
		{name: "connector", manager: &TargetManager{}, want: "target manager connector is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.manager.ResetDatabase(context.Background(), targetLocalConnection(""), "appdb")

			require.Error(t, err)
			assert.EqualError(t, err, tt.want)
		})
	}
}

func TestTargetManagerResetDatabaseRejectsInvalidAndProtectedDatabaseNames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		database string
		want     string
	}{
		{name: "empty", database: " \t", want: "identifier is required"},
		{name: "postgres", database: "postgres", want: "protected database"},
		{name: "template0", database: "template0", want: "protected database"},
		{name: "template1 uppercase", database: "Template1", want: "protected database"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			connector := &targetFakeConnector{conn: &targetFakeConn{}}
			manager := &TargetManager{Connector: connector}

			err := manager.ResetDatabase(context.Background(), targetLocalConnection(""), tt.database)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Empty(t, connector.calls)
		})
	}
}

func TestTargetManagerResetDatabaseWrapsStatementErrorsAndCloses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		execErrs []error
		want     string
	}{
		{name: "terminate", execErrs: []error{errors.New("terminate failed")}, want: "terminate target database sessions"},
		{name: "drop", execErrs: []error{nil, errors.New("drop failed")}, want: "drop target database"},
		{name: "create", execErrs: []error{nil, nil, errors.New("create failed")}, want: "create target database"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			conn := &targetFakeConn{execErrs: append([]error(nil), tt.execErrs...)}
			manager := &TargetManager{Connector: &targetFakeConnector{conn: conn}}

			err := manager.ResetDatabase(context.Background(), targetLocalConnection(""), "appdb")

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.True(t, conn.closed)
		})
	}
}

func TestTargetManagerResetDatabaseReturnsCloseError(t *testing.T) {
	t.Parallel()
	closeErr := errors.New("close failed")
	conn := &targetFakeConn{closeErr: closeErr}
	manager := &TargetManager{Connector: &targetFakeConnector{conn: conn}}

	err := manager.ResetDatabase(context.Background(), targetLocalConnection(""), "appdb")

	require.Error(t, err)
	assert.ErrorIs(t, err, closeErr)
	assert.True(t, conn.closed)
}

func TestApplySQLExecutesMultiStatementSQL(t *testing.T) {
	t.Parallel()
	conn := &targetFakeConn{}

	err := ApplySQL(context.Background(), conn, SchemaPreData, "CREATE TABLE users(id int);")

	require.NoError(t, err)
	assert.Equal(t, []string{"CREATE TABLE users(id int);"}, conn.execMultiSQL)
}

func TestApplySQLWrapsSectionErrors(t *testing.T) {
	t.Parallel()
	execErr := errors.New("syntax error")
	conn := &targetFakeConn{execMultiErr: execErr}

	err := ApplySQL(context.Background(), conn, SchemaPostData, "ALTER TABLE broken")

	require.Error(t, err)
	assert.ErrorIs(t, err, execErr)
	assert.Contains(t, err.Error(), "post-data")
}

func TestApplySQLRejectsUnsupportedSectionAndNilConnection(t *testing.T) {
	t.Parallel()
	conn := &targetFakeConn{}

	err := ApplySQL(context.Background(), conn, SchemaSection("data"), "SELECT 1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported schema section")
	assert.Empty(t, conn.execMultiSQL)

	err = ApplySQL(context.Background(), nil, SchemaPreData, "SELECT 1")
	require.Error(t, err)
	assert.EqualError(t, err, "target connection is required")
}

func targetLocalConnection(database string) config.Connection {
	return config.Connection{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "localpass",
		Database: database,
		SSLMode:  "disable",
	}
}

type targetConnectCall struct {
	endpoint pgdb.Endpoint
}

type targetFakeConnector struct {
	calls []targetConnectCall
	conn  pgdb.CopyConn
	err   error
}

func (c *targetFakeConnector) Connect(_ context.Context, endpoint pgdb.Endpoint) (pgdb.CopyConn, error) {
	c.calls = append(c.calls, targetConnectCall{endpoint: endpoint})
	return c.conn, c.err
}

type targetExecCall struct {
	sql  string
	args []any
}

type targetFakeConn struct {
	execCalls    []targetExecCall
	execErrs     []error
	execMultiSQL []string
	execMultiErr error
	closeErr     error
	closed       bool
}

func (c *targetFakeConn) Query(_ context.Context, _ string, _ ...any) (pgdb.Rows, error) {
	return nil, errors.New("unexpected query")
}

func (c *targetFakeConn) QueryRow(_ context.Context, _ string, _ ...any) pgdb.Row {
	return &targetFakeRow{err: errors.New("unexpected query row")}
}

func (c *targetFakeConn) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	c.execCalls = append(c.execCalls, targetExecCall{sql: sql, args: append([]any(nil), args...)})
	if len(c.execErrs) == 0 {
		return pgconn.CommandTag{}, nil
	}
	err := c.execErrs[0]
	c.execErrs = c.execErrs[1:]
	return pgconn.CommandTag{}, err
}

func (c *targetFakeConn) Close(_ context.Context) error {
	c.closed = true
	return c.closeErr
}

func (c *targetFakeConn) CopyTo(_ context.Context, _ io.Writer, _ string) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unexpected copy to")
}

func (c *targetFakeConn) CopyFrom(_ context.Context, _ io.Reader, _ string) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unexpected copy from")
}

func (c *targetFakeConn) ExecMulti(_ context.Context, sql string) error {
	c.execMultiSQL = append(c.execMultiSQL, sql)
	return c.execMultiErr
}

type targetFakeRow struct {
	err error
}

func (r *targetFakeRow) Scan(_ ...any) error {
	return r.err
}
