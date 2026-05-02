package native

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/pgdb"
)

// RepairSequences advances target sequences after table data has been copied.
func RepairSequences(ctx context.Context, conn pgdb.CopyConn, seqs []models.Sequence) error {
	if conn == nil {
		return errors.New("target connection is required")
	}

	ordered := append([]models.Sequence(nil), seqs...)
	slices.SortFunc(ordered, func(a, b models.Sequence) int {
		return strings.Compare(sequenceRepairSortKey(a), sequenceRepairSortKey(b))
	})

	for _, seq := range ordered {
		sql, err := sequenceRepairSQL(seq)
		if err != nil {
			return sequenceRepairError(seq, err)
		}
		if _, err := conn.Exec(ctx, sql); err != nil {
			return sequenceRepairError(seq, err)
		}
	}
	return nil
}

func sequenceRepairSQL(seq models.Sequence) (string, error) {
	quotedSequence, err := quoteSequenceName(seq)
	if err != nil {
		return "", err
	}
	quotedTable, err := pgdb.QuoteQualified(seq.OwnedTable())
	if err != nil {
		return "", fmt.Errorf("quote owned table: %w", err)
	}
	quotedColumn, err := pgdb.QuoteIdent(seq.ColumnName)
	if err != nil {
		return "", fmt.Errorf("quote column: %w", err)
	}

	return "SELECT setval(" + quoteSQLLiteral(quotedSequence) + "::regclass, COALESCE(MAX(" + quotedColumn + "), 1), MAX(" + quotedColumn + ") IS NOT NULL) FROM " + quotedTable, nil
}

func quoteSequenceName(seq models.Sequence) (string, error) {
	quotedSchema, err := pgdb.QuoteIdent(seq.Schema)
	if err != nil {
		return "", fmt.Errorf("quote sequence schema: %w", err)
	}
	quotedName, err := pgdb.QuoteIdent(seq.Name)
	if err != nil {
		return "", fmt.Errorf("quote sequence name: %w", err)
	}
	return quotedSchema + "." + quotedName, nil
}

func sequenceRepairError(seq models.Sequence, err error) error {
	return fmt.Errorf(
		"repair sequence %s for table %s column %s: %w",
		rawSequenceName(seq),
		rawOwnedTableName(seq),
		seq.ColumnName,
		err,
	)
}

func rawSequenceName(seq models.Sequence) string {
	return seq.Schema + "." + seq.Name
}

func rawOwnedTableName(seq models.Sequence) string {
	return seq.TableSchema + "." + seq.TableName
}

func sequenceRepairSortKey(seq models.Sequence) string {
	return strings.Join([]string{seq.Schema, seq.Name, seq.TableSchema, seq.TableName, seq.ColumnName}, "\x00")
}
