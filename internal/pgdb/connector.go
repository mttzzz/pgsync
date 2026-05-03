package pgdb

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/proxy"
)

type copyConnOpener func(ctx context.Context, connString string) (CopyConn, error)
type nativeOpener func(ctx context.Context, connString string) (*pgx.Conn, error)

// ContextDialer is the network dialer shape accepted by pgx/pgconn.
type ContextDialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

type connOperations struct {
	query     func(ctx context.Context, sql string, args ...any) (Rows, error)
	queryRow  func(ctx context.Context, sql string, args ...any) Row
	exec      func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	close     func(ctx context.Context) error
	copyTo    func(ctx context.Context, w io.Writer, sql string) (pgconn.CommandTag, error)
	copyFrom  func(ctx context.Context, r io.Reader, sql string) (pgconn.CommandTag, error)
	execMulti func(ctx context.Context, sql string) error
}

type multiResultReader interface {
	ReadAll() ([]*pgconn.Result, error)
}

/* PgxConnector opens production pgx connections. */
type PgxConnector struct {
	open copyConnOpener
}

/* NewConnector returns the default PostgreSQL connector. */
func NewConnector() Connector {
	return NewPgxConnector()
}

/* NewPgxConnector returns a connector backed by pgx.Connect. */
func NewPgxConnector() *PgxConnector {
	return &PgxConnector{open: openPgxConn}
}

/* NewPgxConnectorWithDialer returns a connector backed by pgx with a custom network dialer. */
func NewPgxConnectorWithDialer(dialer ContextDialer) *PgxConnector {
	if dialer == nil {
		return NewPgxConnector()
	}
	return &PgxConnector{open: func(ctx context.Context, connString string) (CopyConn, error) {
		return openPgxConnWithDialer(ctx, connString, dialer)
	}}
}

/* Connect opens an Endpoint with pgx and redacts the DSN in returned errors. */
func (c *PgxConnector) Connect(ctx context.Context, ep Endpoint) (CopyConn, error) {
	connString, err := BuildConnString(ep)
	if err != nil {
		return nil, fmt.Errorf("build pg connection string: %w", err)
	}
	open := c.open
	if ep.ProxyURL != "" {
		dialer, dialErr := proxy.NewDialer(ep.ProxyURL)
		if dialErr != nil {
			return nil, fmt.Errorf("init proxy: %w", dialErr)
		}
		open = func(ctx context.Context, connString string) (CopyConn, error) {
			return openPgxConnWithDialer(ctx, connString, dialer)
		}
	}
	if open == nil {
		open = openPgxConn
	}
	conn, err := open(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("connect postgres %s: %w", MaskConnString(connString), err)
	}
	return conn, nil
}

/* ConfigConnector opens config.Connection values after applying database
 * overrides.
 */
type ConfigConnector struct {
	connector Connector
}

/* NewConfigConnector returns a connector for unresolved config.Connection
 * values.
 */
func NewConfigConnector(connector Connector) *ConfigConnector {
	return &ConfigConnector{connector: connector}
}

/* Connect applies databaseOverride exactly and delegates to the wrapped
 * Connector.
 */
func (c *ConfigConnector) Connect(
	ctx context.Context,
	cfg config.Connection,
	databaseOverride string,
) (CopyConn, error) {
	return c.connector.Connect(ctx, EndpointFromConfig(cfg, databaseOverride))
}

/* PgxConn adapts a pgx.Conn to CopyConn and makes Close idempotent. */
type PgxConn struct {
	native *pgx.Conn
	ops    *connOperations
	mu     sync.Mutex
	closed bool
}

/* NewPgxConn adapts conn to the CopyConn interface. */
func NewPgxConn(conn *pgx.Conn) *PgxConn {
	return &PgxConn{native: conn}
}

func newPgxConnWithOperations(ops connOperations) *PgxConn {
	return &PgxConn{ops: &ops}
}

/* Query delegates to the wrapped pgx connection. */
func (c *PgxConn) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	if c.ops != nil {
		return c.ops.query(ctx, sql, args...)
	}
	return c.native.Query(ctx, sql, args...)
}

/* QueryRow delegates to the wrapped pgx connection. */
func (c *PgxConn) QueryRow(ctx context.Context, sql string, args ...any) Row {
	if c.ops != nil {
		return c.ops.queryRow(ctx, sql, args...)
	}
	return c.native.QueryRow(ctx, sql, args...)
}

/* Exec delegates to the wrapped pgx connection. */
func (c *PgxConn) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if c.ops != nil {
		return c.ops.exec(ctx, sql, args...)
	}
	return c.native.Exec(ctx, sql, args...)
}

/* Close closes the wrapped connection at most once. */
func (c *PgxConn) Close(ctx context.Context) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	if c.ops != nil {
		return c.ops.close(ctx)
	}
	return c.native.Close(ctx)
}

/* CopyTo delegates binary COPY TO STDOUT to pgconn. */
func (c *PgxConn) CopyTo(ctx context.Context, w io.Writer, sql string) (pgconn.CommandTag, error) {
	if c.ops != nil {
		return c.ops.copyTo(ctx, w, sql)
	}
	return c.native.PgConn().CopyTo(ctx, w, sql)
}

/* CopyFrom delegates binary COPY FROM STDIN to pgconn. */
func (c *PgxConn) CopyFrom(ctx context.Context, r io.Reader, sql string) (pgconn.CommandTag, error) {
	if c.ops != nil {
		return c.ops.copyFrom(ctx, r, sql)
	}
	return c.native.PgConn().CopyFrom(ctx, r, sql)
}

/* ExecMulti executes multi-statement SQL through pgconn's simple protocol. */
func (c *PgxConn) ExecMulti(ctx context.Context, sql string) error {
	if c.ops != nil {
		return c.ops.execMulti(ctx, sql)
	}
	return readAllResults(c.native.PgConn().Exec(ctx, sql))
}

func openPgxConn(ctx context.Context, connString string) (CopyConn, error) {
	return openPgxConnWith(ctx, connString, pgx.Connect)
}

func openPgxConnWithDialer(ctx context.Context, connString string, dialer ContextDialer) (CopyConn, error) {
	cfg, err := pgx.ParseConfig(connString)
	if err != nil {
		return nil, err
	}
	cfg.DialFunc = dialer.DialContext
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return NewPgxConn(conn), nil
}

func openPgxConnWith(ctx context.Context, connString string, open nativeOpener) (CopyConn, error) {
	conn, err := open(ctx, connString)
	if err != nil {
		return nil, err
	}
	return NewPgxConn(conn), nil
}

func readAllResults(reader multiResultReader) error {
	_, err := reader.ReadAll()
	return err
}
