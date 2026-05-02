package pgschema_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/pgdb"
	"github.com/mttzzz/pgsync/internal/pgschema"
)

func TestServiceListDatabasesScansDatabases(t *testing.T) {
	t.Parallel()
	rows := newFakeRows([]any{"app", int64(2048), "owner"}, []any{"analytics", int64(4096), "dba"})
	querier := &fakeQuerier{rows: rows}

	got, err := pgschema.NewService(querier).ListDatabases(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []models.Database{
		{Name: "app", SizeBytes: 2048, Owner: "owner"},
		{Name: "analytics", SizeBytes: 4096, Owner: "dba"},
	}, got)
	assert.Equal(t, pgschema.ListDatabasesSQL(), querier.sql)
	assert.True(t, rows.closed)
}

func TestServiceListTablesScansTableMetadata(t *testing.T) {
	t.Parallel()
	rows := newFakeRows([]any{"public", "users", int64(8192), int64(12)})
	querier := &fakeQuerier{rows: rows}

	got, err := pgschema.NewService(querier).ListTables(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []models.Table{{Schema: "public", Name: "users", SizeBytes: 8192, Rows: 12}}, got)
	assert.Equal(t, pgschema.ListTablesSQL(), querier.sql)
	assert.True(t, rows.closed)
}

func TestServiceListFKDepsMapsChildToParent(t *testing.T) {
	t.Parallel()
	rows := newFakeRows([]any{"sales", "orders", "public", "users"})
	querier := &fakeQuerier{rows: rows}

	got, err := pgschema.NewService(querier).ListFKDeps(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []models.FKDep{{
		From: models.Table{Schema: "sales", Name: "orders"},
		To:   models.Table{Schema: "public", Name: "users"},
	}}, got)
	assert.Equal(t, pgschema.ListFKDepsSQL(), querier.sql)
	assert.True(t, rows.closed)
}

func TestServiceListSequencesMapsOwnership(t *testing.T) {
	t.Parallel()
	rows := newFakeRows([]any{"public", "users_id_seq", "public", "users", "id"})
	querier := &fakeQuerier{rows: rows}

	got, err := pgschema.NewService(querier).ListSequences(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []models.Sequence{{
		Schema:      "public",
		Name:        "users_id_seq",
		TableSchema: "public",
		TableName:   "users",
		ColumnName:  "id",
	}}, got)
	assert.Equal(t, pgschema.ListSequencesSQL(), querier.sql)
	assert.True(t, rows.closed)
}

func TestServiceQueryErrorPropagatesWithOperationName(t *testing.T) {
	t.Parallel()
	queryErr := errors.New("query failed")
	querier := &fakeQuerier{err: queryErr}

	_, err := pgschema.NewService(querier).ListDatabases(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, queryErr)
	assert.Contains(t, err.Error(), "list databases")
}

func TestServiceScanErrorClosesRowsAndPropagates(t *testing.T) {
	t.Parallel()
	scanErr := errors.New("scan failed")
	rows := newFakeRows([]any{"app", int64(1), "owner"})
	rows.scanErr = scanErr
	querier := &fakeQuerier{rows: rows}

	_, err := pgschema.NewService(querier).ListDatabases(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, scanErr)
	assert.Contains(t, err.Error(), "list databases: scan")
	assert.True(t, rows.closed)
}

func TestServiceRowsErrAfterIterationPropagates(t *testing.T) {
	t.Parallel()
	rowsErr := errors.New("rows failed")
	rows := newFakeRows()
	rows.err = rowsErr
	querier := &fakeQuerier{rows: rows}

	_, err := pgschema.NewService(querier).ListDatabases(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, rowsErr)
	assert.Contains(t, err.Error(), "list databases: rows")
	assert.True(t, rows.closed)
}

type fakeQuerier struct {
	rows pgdb.Rows
	err  error
	sql  string
}

func (q *fakeQuerier) Query(_ context.Context, sql string, _ ...any) (pgdb.Rows, error) {
	q.sql = sql
	return q.rows, q.err
}

func (q *fakeQuerier) QueryRow(_ context.Context, _ string, _ ...any) pgdb.Row {
	return fakeRow{}
}

func (q *fakeQuerier) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

type fakeRow struct{}

func (fakeRow) Scan(_ ...any) error {
	return nil
}

type fakeRows struct {
	records [][]any
	next    int
	scanErr error
	err     error
	closed  bool
}

func newFakeRows(records ...[]any) *fakeRows {
	return &fakeRows{records: records}
}

func (r *fakeRows) Close() {
	r.closed = true
}

func (r *fakeRows) Err() error {
	return r.err
}

func (r *fakeRows) Next() bool {
	if r.next >= len(r.records) {
		return false
	}
	r.next++
	return true
}

func (r *fakeRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	values := r.records[r.next-1]
	if len(dest) != len(values) {
		return fmt.Errorf("scan destination count %d does not match value count %d", len(dest), len(values))
	}
	for i, target := range dest {
		if err := assignScanValue(target, values[i]); err != nil {
			return fmt.Errorf("scan value %d: %w", i, err)
		}
	}
	return nil
}

func assignScanValue(target any, value any) error {
	switch typed := target.(type) {
	case *string:
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
		*typed = text
	case *int64:
		number, ok := value.(int64)
		if !ok {
			return fmt.Errorf("expected int64, got %T", value)
		}
		*typed = number
	default:
		return fmt.Errorf("unsupported scan target %T", target)
	}
	return nil
}
