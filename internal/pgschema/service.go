package pgschema

import (
	"context"
	"fmt"

	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/pgdb"
)

/* Service reads PostgreSQL catalog metadata through a pgdb querier. */
type Service struct {
	q pgdb.Querier
}

/* NewService returns a catalog service backed by q. */
func NewService(q pgdb.Querier) *Service {
	return &Service{q: q}
}

/* ListDatabases lists non-template, non-system databases. */
func (s *Service) ListDatabases(ctx context.Context) ([]models.Database, error) {
	return queryCatalog(ctx, s.q, "list databases", ListDatabasesSQL(), scanDatabase)
}

/* ListTables lists ordinary and partitioned tables in user schemas. */
func (s *Service) ListTables(ctx context.Context) ([]models.Table, error) {
	return queryCatalog(ctx, s.q, "list tables", ListTablesSQL(), scanTable)
}

/* CountRows returns the exact row count for a table via SELECT count(*).
 * quotedRelation must already be safely quoted (use pgdb.QuoteQualified). */
func (s *Service) CountRows(ctx context.Context, quotedRelation string) (int64, error) {
	var n int64
	if err := s.q.QueryRow(ctx, CountRowsSQL(quotedRelation)).Scan(&n); err != nil {
		return 0, fmt.Errorf("count rows in %s: %w", quotedRelation, err)
	}
	return n, nil
}

/* ListFKDeps lists foreign-key edges from child tables to parent tables. */
func (s *Service) ListFKDeps(ctx context.Context) ([]models.FKDep, error) {
	return queryCatalog(ctx, s.q, "list foreign keys", ListFKDepsSQL(), scanFKDep)
}

/* ListSequences lists owned sequences and their table column ownership. */
func (s *Service) ListSequences(ctx context.Context) ([]models.Sequence, error) {
	return queryCatalog(ctx, s.q, "list sequences", ListSequencesSQL(), scanSequence)
}

func queryCatalog[T any](
	ctx context.Context,
	q pgdb.Querier,
	operation string,
	sql string,
	scan func(pgdb.Rows) (T, error),
) ([]T, error) {
	rows, err := q.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()

	out := make([]T, 0)
	for rows.Next() {
		item, scanErr := scan(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("%s: scan: %w", operation, scanErr)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: rows: %w", operation, err)
	}
	return out, nil
}

func scanDatabase(rows pgdb.Rows) (models.Database, error) {
	var database models.Database
	err := rows.Scan(&database.Name, &database.SizeBytes, &database.Owner)
	return database, err
}

func scanTable(rows pgdb.Rows) (models.Table, error) {
	var table models.Table
	err := rows.Scan(&table.Schema, &table.Name, &table.SizeBytes, &table.Rows)
	return table, err
}

func scanFKDep(rows pgdb.Rows) (models.FKDep, error) {
	var dep models.FKDep
	err := rows.Scan(&dep.From.Schema, &dep.From.Name, &dep.To.Schema, &dep.To.Name)
	return dep, err
}

func scanSequence(rows pgdb.Rows) (models.Sequence, error) {
	var sequence models.Sequence
	err := rows.Scan(
		&sequence.Schema,
		&sequence.Name,
		&sequence.TableSchema,
		&sequence.TableName,
		&sequence.ColumnName,
	)
	return sequence, err
}
