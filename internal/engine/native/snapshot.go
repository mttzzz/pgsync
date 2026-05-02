// Package native implements Go-native PostgreSQL synchronization primitives.
package native

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mttzzz/pgsync/internal/pgdb"
)

const (
	beginRepeatableReadSQL = "BEGIN ISOLATION LEVEL REPEATABLE READ"
	exportSnapshotSQL      = "SELECT pg_export_snapshot()"
	rollbackSQL            = "ROLLBACK"
)

// Snapshot is an exported PostgreSQL transaction snapshot held open by its
// source connection.
type Snapshot struct {
	ID   string
	Conn pgdb.CopyConn
}

// ExportSnapshot starts a repeatable-read transaction and exports a PostgreSQL
// snapshot ID that can be adopted by worker transactions.
func ExportSnapshot(ctx context.Context, conn pgdb.CopyConn) (*Snapshot, error) {
	if _, err := conn.Exec(ctx, beginRepeatableReadSQL); err != nil {
		return nil, fmt.Errorf("begin snapshot transaction: %w", err)
	}

	var snapshotID string
	if err := conn.QueryRow(ctx, exportSnapshotSQL).Scan(&snapshotID); err != nil {
		return nil, fmt.Errorf(
			"export snapshot: %w",
			errors.Join(err, rollbackAndClose(context.WithoutCancel(ctx), conn)),
		)
	}

	return &Snapshot{ID: snapshotID, Conn: conn}, nil
}

// ApplySnapshot starts a repeatable-read transaction and adopts an exported
// snapshot before any table-copy query can run on the connection.
func ApplySnapshot(ctx context.Context, conn pgdb.CopyConn, snapshotID string) error {
	if _, err := conn.Exec(ctx, beginRepeatableReadSQL); err != nil {
		return fmt.Errorf("begin snapshot transaction: %w", err)
	}

	if _, err := conn.Exec(ctx, "SET TRANSACTION SNAPSHOT "+quoteSQLLiteral(snapshotID)); err != nil {
		return fmt.Errorf(
			"set transaction snapshot: %w",
			errors.Join(err, rollback(context.WithoutCancel(ctx), conn)),
		)
	}

	return nil
}

// Close rolls back the source snapshot transaction and closes its connection.
func (s *Snapshot) Close(ctx context.Context) error {
	return rollbackAndClose(ctx, s.Conn)
}

func rollback(ctx context.Context, conn pgdb.CopyConn) error {
	_, err := conn.Exec(ctx, rollbackSQL)
	return err
}

func rollbackAndClose(ctx context.Context, conn pgdb.CopyConn) error {
	rollbackErr := rollback(ctx, conn)
	closeErr := conn.Close(ctx)
	return errors.Join(rollbackErr, closeErr)
}

func quoteSQLLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
