package pgdb

import (
	"context"
	"io"

	"github.com/jackc/pgx/v5/pgconn"
)

/* Rows is the subset of pgx.Rows used by the native engine. */
type Rows interface {
	Close()
	Err() error
	Next() bool
	Scan(dest ...any) error
}

/* Querier is the query and exec subset shared by PostgreSQL connections. */
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

/* Row is the subset of pgx.Row used by the native engine. */
type Row interface {
	Scan(dest ...any) error
}

/* Conn is a closeable PostgreSQL connection. */
type Conn interface {
	Querier
	Close(ctx context.Context) error
}

/* CopyConn is a PostgreSQL connection capable of binary COPY and multi-statement
 * execution.
 */
type CopyConn interface {
	Conn
	CopyTo(ctx context.Context, w io.Writer, sql string) (pgconn.CommandTag, error)
	CopyFrom(ctx context.Context, r io.Reader, sql string) (pgconn.CommandTag, error)
	ExecMulti(ctx context.Context, sql string) error
}

/* Connector opens PostgreSQL connections. */
type Connector interface {
	Connect(ctx context.Context, ep Endpoint) (CopyConn, error)
}
