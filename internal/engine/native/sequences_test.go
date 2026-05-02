package native

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/pgdb"
)

func TestRepairSequencesExecutesQuotedSetvalInDeterministicOrder(t *testing.T) {
	t.Parallel()
	conn := &sequenceFakeConn{}
	seqs := []models.Sequence{
		{Schema: "sales", Name: "orders_id_seq", TableSchema: "sales", TableName: "orders", ColumnName: "id"},
		{Schema: `app"odd`, Name: `thing'seq`, TableSchema: `app"odd`, TableName: "weird table", ColumnName: `id"col`},
		{Schema: "app", Name: "accounts_id_seq", TableSchema: "app", TableName: "accounts", ColumnName: "id"},
	}
	original := append([]models.Sequence(nil), seqs...)

	err := RepairSequences(context.Background(), conn, seqs)

	require.NoError(t, err)
	assert.Equal(t, original, seqs)
	assert.Equal(t, []sequenceExecCall{
		{sql: `SELECT setval('"app"."accounts_id_seq"'::regclass, COALESCE(MAX("id"), 1), MAX("id") IS NOT NULL) FROM "app"."accounts"`},
		{sql: `SELECT setval('"app""odd"."thing''seq"'::regclass, COALESCE(MAX("id""col"), 1), MAX("id""col") IS NOT NULL) FROM "app""odd"."weird table"`},
		{sql: `SELECT setval('"sales"."orders_id_seq"'::regclass, COALESCE(MAX("id"), 1), MAX("id") IS NOT NULL) FROM "sales"."orders"`},
	}, conn.execCalls)
}

func TestRepairSequencesAcceptsEmptySequenceList(t *testing.T) {
	t.Parallel()
	conn := &sequenceFakeConn{}

	err := RepairSequences(context.Background(), conn, nil)

	require.NoError(t, err)
	assert.Empty(t, conn.execCalls)
}

func TestRepairSequencesRequiresConnection(t *testing.T) {
	t.Parallel()

	err := RepairSequences(context.Background(), nil, nil)

	require.EqualError(t, err, "target connection is required")
}

func TestRepairSequencesExecErrorIncludesSequenceTableAndColumnAndStops(t *testing.T) {
	t.Parallel()
	execErr := errors.New("setval failed")
	conn := &sequenceFakeConn{execErrs: []error{nil, execErr, nil}}
	seqs := []models.Sequence{
		{Schema: "public", Name: "c_seq", TableSchema: "public", TableName: "c", ColumnName: "id"},
		{Schema: "public", Name: "a_seq", TableSchema: "public", TableName: "a", ColumnName: "id"},
		{Schema: "public", Name: "b_seq", TableSchema: "public", TableName: "b", ColumnName: "id"},
	}

	err := RepairSequences(context.Background(), conn, seqs)

	require.Error(t, err)
	assert.ErrorIs(t, err, execErr)
	assert.Contains(t, err.Error(), "repair sequence public.b_seq for table public.b column id")
	require.Len(t, conn.execCalls, 2)
	assert.Contains(t, conn.execCalls[0].sql, `'"public"."a_seq"'::regclass`)
	assert.Contains(t, conn.execCalls[1].sql, `'"public"."b_seq"'::regclass`)
}

func TestRepairSequencesQuoteErrorsIncludeSequenceTableAndStop(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		seq  models.Sequence
		want string
	}{
		{
			name: "sequence schema",
			seq:  models.Sequence{Schema: " ", Name: "users_id_seq", TableSchema: "public", TableName: "users", ColumnName: "id"},
			want: "quote sequence schema",
		},
		{
			name: "sequence name",
			seq:  models.Sequence{Schema: "public", Name: "", TableSchema: "public", TableName: "users", ColumnName: "id"},
			want: "quote sequence name",
		},
		{
			name: "table schema",
			seq:  models.Sequence{Schema: "public", Name: "users_id_seq", TableSchema: "", TableName: "users", ColumnName: "id"},
			want: "quote owned table: quote schema",
		},
		{
			name: "table name",
			seq:  models.Sequence{Schema: "public", Name: "users_id_seq", TableSchema: "public", TableName: "", ColumnName: "id"},
			want: "quote owned table: quote table",
		},
		{
			name: "column",
			seq:  models.Sequence{Schema: "public", Name: "users_id_seq", TableSchema: "public", TableName: "users", ColumnName: "\t"},
			want: "quote column",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			conn := &sequenceFakeConn{}

			err := RepairSequences(context.Background(), conn, []models.Sequence{tt.seq})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "repair sequence "+tt.seq.Schema+"."+tt.seq.Name)
			assert.Contains(t, err.Error(), "for table "+tt.seq.TableSchema+"."+tt.seq.TableName+" column "+tt.seq.ColumnName)
			assert.Contains(t, err.Error(), tt.want)
			assert.Empty(t, conn.execCalls)
		})
	}
}

type sequenceExecCall struct {
	sql  string
	args []any
}

type sequenceFakeConn struct {
	execCalls []sequenceExecCall
	execErrs  []error
}

func (c *sequenceFakeConn) Query(_ context.Context, _ string, _ ...any) (pgdb.Rows, error) {
	return nil, errors.New("unexpected query")
}

func (c *sequenceFakeConn) QueryRow(_ context.Context, _ string, _ ...any) pgdb.Row {
	return &sequenceFakeRow{err: errors.New("unexpected query row")}
}

func (c *sequenceFakeConn) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	c.execCalls = append(c.execCalls, sequenceExecCall{sql: sql, args: append([]any(nil), args...)})
	if len(c.execErrs) == 0 {
		return pgconn.CommandTag{}, nil
	}
	err := c.execErrs[0]
	c.execErrs = c.execErrs[1:]
	return pgconn.CommandTag{}, err
}

func (c *sequenceFakeConn) Close(_ context.Context) error {
	return nil
}

func (c *sequenceFakeConn) CopyTo(_ context.Context, _ io.Writer, _ string) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unexpected copy to")
}

func (c *sequenceFakeConn) CopyFrom(_ context.Context, _ io.Reader, _ string) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unexpected copy from")
}

func (c *sequenceFakeConn) ExecMulti(_ context.Context, _ string) error {
	return errors.New("unexpected exec multi")
}

type sequenceFakeRow struct {
	err error
}

func (r *sequenceFakeRow) Scan(_ ...any) error {
	return r.err
}
