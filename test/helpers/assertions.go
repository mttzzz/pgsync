package helpers

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/pgdb"
)

const defaultFixtureSchema = "public"

type tableRef interface {
	string | models.Table
}

type sequenceColumn struct {
	ColumnName   string
	SequenceName string
}

// AssertTableRowCountsEqual verifies that all selected tables have equal row counts.
func AssertTableRowCountsEqual[T tableRef](
	ctx context.Context,
	t testing.TB,
	source PostgresContainer,
	target PostgresContainer,
	tables []T,
) {
	t.Helper()

	parsedTables := parseTables(t, tables)
	sourceConn := connectPostgres(ctx, t, source)
	defer closePostgres(ctx, t, sourceConn)
	targetConn := connectPostgres(ctx, t, target)
	defer closePostgres(ctx, t, targetConn)

	for _, table := range parsedTables {
		sourceCount := queryTableRowCount(ctx, t, sourceConn, table)
		targetCount := queryTableRowCount(ctx, t, targetConn, table)
		if sourceCount != targetCount {
			t.Fatalf("row count mismatch for %s: source=%d target=%d", tableName(table), sourceCount, targetCount)
		}
	}
}

// AssertTableChecksumsEqual verifies deterministic per-table content checksums.
func AssertTableChecksumsEqual[T tableRef](
	ctx context.Context,
	t testing.TB,
	source PostgresContainer,
	target PostgresContainer,
	tables []T,
) {
	t.Helper()

	parsedTables := parseTables(t, tables)
	sourceConn := connectPostgres(ctx, t, source)
	defer closePostgres(ctx, t, sourceConn)
	targetConn := connectPostgres(ctx, t, target)
	defer closePostgres(ctx, t, targetConn)

	for _, table := range parsedTables {
		sourceChecksum := queryTableChecksum(ctx, t, sourceConn, table)
		targetChecksum := queryTableChecksum(ctx, t, targetConn, table)
		if sourceChecksum != targetChecksum {
			t.Fatalf("checksum mismatch for %s: source=%s target=%s", tableName(table), sourceChecksum, targetChecksum)
		}
	}
}

// AssertSequencesUsable verifies that table-owned sequences advance beyond existing IDs.
func AssertSequencesUsable[T tableRef](ctx context.Context, t testing.TB, target PostgresContainer, tables []T) {
	t.Helper()

	parsedTables := parseTables(t, tables)
	targetConn := connectPostgres(ctx, t, target)
	defer closePostgres(ctx, t, targetConn)

	for _, table := range parsedTables {
		columns := querySequenceColumns(ctx, t, targetConn, table)
		for _, column := range columns {
			maxID := queryMaxColumnValue(ctx, t, targetConn, table, column.ColumnName)
			nextID := queryNextSequenceValue(ctx, t, targetConn, column.SequenceName)
			if nextID <= maxID {
				t.Fatalf(
					"sequence %s for %s.%s is not usable: nextval=%d max=%d",
					column.SequenceName,
					tableName(table),
					column.ColumnName,
					nextID,
					maxID,
				)
			}
		}
	}
}

// AssertIndexExists verifies that a PostgreSQL index with indexName exists.
func AssertIndexExists(ctx context.Context, t testing.TB, target PostgresContainer, indexName string) {
	t.Helper()

	name := strings.TrimSpace(indexName)
	if name == "" {
		t.Fatal("index name is required")
	}

	conn := connectPostgres(ctx, t, target)
	defer closePostgres(ctx, t, conn)

	var exists bool
	if err := conn.QueryRow(ctx, indexExistsQuery, name).Scan(&exists); err != nil {
		t.Fatalf("check index %q exists: %v", name, err)
	}
	if !exists {
		t.Fatalf("expected index %q to exist", name)
	}
}

// AssertFKExists verifies that a PostgreSQL foreign key constraint exists.
func AssertFKExists(ctx context.Context, t testing.TB, target PostgresContainer, constraintName string) {
	t.Helper()

	name := strings.TrimSpace(constraintName)
	if name == "" {
		t.Fatal("foreign key constraint name is required")
	}

	conn := connectPostgres(ctx, t, target)
	defer closePostgres(ctx, t, conn)

	var exists bool
	if err := conn.QueryRow(ctx, fkExistsQuery, name).Scan(&exists); err != nil {
		t.Fatalf("check foreign key %q exists: %v", name, err)
	}
	if !exists {
		t.Fatalf("expected foreign key %q to exist", name)
	}
}

func parseTables[T tableRef](t testing.TB, tables []T) []models.Table {
	t.Helper()

	if len(tables) == 0 {
		t.Fatal("at least one table is required")
	}

	parsed := make([]models.Table, 0, len(tables))
	for _, table := range tables {
		parsedTable, err := parseTable(table)
		if err != nil {
			t.Fatalf("parse table reference: %v", err)
		}
		parsed = append(parsed, parsedTable)
	}
	return parsed
}

func parseTable[T tableRef](table T) (models.Table, error) {
	switch value := any(table).(type) {
	case string:
		return pgdb.ParseTableName(value, defaultFixtureSchema)
	case models.Table:
		if strings.TrimSpace(value.Schema) == "" {
			return models.Table{}, fmt.Errorf("table schema is required for %q", value.Name)
		}
		if strings.TrimSpace(value.Name) == "" {
			return models.Table{}, fmt.Errorf("table name is required for schema %q", value.Schema)
		}
		return value, nil
	default:
		return models.Table{}, fmt.Errorf("unsupported table reference %T", table)
	}
}

func queryTableRowCount(ctx context.Context, t testing.TB, conn *pgx.Conn, table models.Table) int64 {
	t.Helper()

	query, err := tableRowCountQuery(table)
	if err != nil {
		t.Fatalf("build row count query for %s: %v", tableName(table), err)
	}

	var count int64
	if err := conn.QueryRow(ctx, query).Scan(&count); err != nil {
		t.Fatalf("query row count for %s: %v", tableName(table), err)
	}
	return count
}

func queryTableChecksum(ctx context.Context, t testing.TB, conn *pgx.Conn, table models.Table) string {
	t.Helper()

	query, err := tableChecksumQuery(table)
	if err != nil {
		t.Fatalf("build checksum query for %s: %v", tableName(table), err)
	}

	var checksum string
	if err := conn.QueryRow(ctx, query).Scan(&checksum); err != nil {
		t.Fatalf("query checksum for %s: %v", tableName(table), err)
	}
	return checksum
}

func querySequenceColumns(ctx context.Context, t testing.TB, conn *pgx.Conn, table models.Table) []sequenceColumn {
	t.Helper()

	rows, err := conn.Query(ctx, sequenceColumnsQuery, table.Schema, table.Name)
	if err != nil {
		t.Fatalf("query sequence columns for %s: %v", tableName(table), err)
	}
	defer rows.Close()

	var columns []sequenceColumn
	for rows.Next() {
		var columnName string
		var sequenceName sql.NullString
		if err := rows.Scan(&columnName, &sequenceName); err != nil {
			t.Fatalf("scan sequence column for %s: %v", tableName(table), err)
		}
		if sequenceName.Valid && sequenceName.String != "" {
			columns = append(columns, sequenceColumn{ColumnName: columnName, SequenceName: sequenceName.String})
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sequence columns for %s: %v", tableName(table), err)
	}
	return columns
}

func queryMaxColumnValue(ctx context.Context, t testing.TB, conn *pgx.Conn, table models.Table, column string) int64 {
	t.Helper()

	query, err := tableMaxColumnValueQuery(table, column)
	if err != nil {
		t.Fatalf("build max column query for %s.%s: %v", tableName(table), column, err)
	}

	var maxID int64
	if err := conn.QueryRow(ctx, query).Scan(&maxID); err != nil {
		t.Fatalf("query max column value for %s.%s: %v", tableName(table), column, err)
	}
	return maxID
}

func queryNextSequenceValue(ctx context.Context, t testing.TB, conn *pgx.Conn, sequenceName string) int64 {
	t.Helper()

	var nextID int64
	if err := conn.QueryRow(ctx, nextSequenceValueQuery, sequenceName).Scan(&nextID); err != nil {
		t.Fatalf("query next sequence value for %s: %v", sequenceName, err)
	}
	return nextID
}

func tableRowCountQuery(table models.Table) (string, error) {
	quotedTable, err := pgdb.QuoteQualified(table)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("SELECT count(*)::bigint FROM %s", quotedTable), nil
}

func tableChecksumQuery(table models.Table) (string, error) {
	quotedTable, err := pgdb.QuoteQualified(table)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"SELECT COALESCE(md5(string_agg(row_json::text, E'\\n' ORDER BY row_json::text)), md5('')) FROM (SELECT to_jsonb(t) AS row_json FROM %s AS t) AS rows",
		quotedTable,
	), nil
}

func tableMaxColumnValueQuery(table models.Table, column string) (string, error) {
	quotedTable, err := pgdb.QuoteQualified(table)
	if err != nil {
		return "", err
	}
	quotedColumn, err := pgdb.QuoteIdent(column)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("SELECT COALESCE(max(%s), 0)::bigint FROM %s", quotedColumn, quotedTable), nil
}

func tableName(table models.Table) string {
	return table.Schema + "." + table.Name
}

const indexExistsQuery = `
SELECT EXISTS (
    SELECT 1
    FROM pg_catalog.pg_class c
    WHERE c.relkind = 'i'
      AND c.relname = $1
)`

const fkExistsQuery = `
SELECT EXISTS (
    SELECT 1
    FROM pg_catalog.pg_constraint con
    WHERE con.contype = 'f'
      AND con.conname = $1
)`

const sequenceColumnsQuery = `
SELECT
    a.attname,
    pg_catalog.pg_get_serial_sequence(format('%I.%I', n.nspname, c.relname), a.attname)
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
JOIN pg_catalog.pg_attribute a ON a.attrelid = c.oid
WHERE n.nspname = $1
  AND c.relname = $2
  AND c.relkind IN ('r', 'p')
  AND a.attnum > 0
  AND NOT a.attisdropped
  AND (
      a.attidentity <> ''
      OR pg_catalog.pg_get_serial_sequence(format('%I.%I', n.nspname, c.relname), a.attname) IS NOT NULL
  )
ORDER BY a.attnum`

const nextSequenceValueQuery = "SELECT nextval($1::regclass)::bigint"
