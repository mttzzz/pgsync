package native

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/pgdb"
)

const terminateTargetSessionsSQL = "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()"

// TargetManager manages destructive setup of the local target database.
type TargetManager struct {
	Connector pgdb.Connector
}

// ResetDatabase terminates sessions connected to database, drops it when it
// exists, then recreates it by connecting to the local maintenance database.
func (m *TargetManager) ResetDatabase(ctx context.Context, local config.Connection, database string) (err error) {
	if m == nil {
		return errors.New("target manager is required")
	}
	if m.Connector == nil {
		return errors.New("target manager connector is required")
	}

	targetDatabase, quotedTargetDatabase, err := quoteResetDatabaseName(database)
	if err != nil {
		return err
	}

	endpoint := pgdb.EndpointFromConfig(local, maintenanceDatabase(local))
	conn, err := m.Connector.Connect(ctx, endpoint)
	if err != nil {
		return targetConnectError(err, endpoint)
	}
	defer func() {
		closeErr := conn.Close(context.WithoutCancel(ctx))
		if closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close target maintenance connection: %w", closeErr))
		}
	}()

	return executeReset(ctx, conn, targetDatabase, quotedTargetDatabase)
}

// ApplySQL applies one schema SQL section with PostgreSQL's simple protocol.
func ApplySQL(ctx context.Context, conn pgdb.CopyConn, section SchemaSection, sql string) error {
	if err := validateSchemaSection(section); err != nil {
		return err
	}
	if conn == nil {
		return errors.New("target connection is required")
	}
	if err := conn.ExecMulti(ctx, sql); err != nil {
		return fmt.Errorf("apply %s schema SQL: %w", section, err)
	}
	return nil
}

func maintenanceDatabase(local config.Connection) string {
	if strings.TrimSpace(local.Database) == "" {
		return "postgres"
	}
	return local.Database
}

func quoteResetDatabaseName(database string) (string, string, error) {
	trimmed := strings.TrimSpace(database)
	quoted, err := pgdb.QuoteIdent(trimmed)
	if err != nil {
		return "", "", fmt.Errorf("target database: %w", err)
	}
	if isProtectedDatabase(trimmed) {
		return "", "", fmt.Errorf("refusing to reset protected database %q", trimmed)
	}
	return trimmed, quoted, nil
}

func isProtectedDatabase(database string) bool {
	switch strings.ToLower(database) {
	case "postgres", "template0", "template1":
		return true
	default:
		return false
	}
}

func targetConnectError(err error, endpoint pgdb.Endpoint) error {
	return fmt.Errorf(
		"connect target maintenance database %q: %s",
		endpoint.Database,
		redactEndpointText(err.Error(), endpoint, ""),
	)
}

func executeReset(ctx context.Context, conn pgdb.CopyConn, database string, quotedDatabase string) error {
	if _, err := conn.Exec(ctx, terminateTargetSessionsSQL, database); err != nil {
		return fmt.Errorf("terminate target database sessions %q: %w", database, err)
	}
	if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+quotedDatabase); err != nil {
		return fmt.Errorf("drop target database %s: %w", quotedDatabase, err)
	}
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+quotedDatabase); err != nil {
		return fmt.Errorf("create target database %s: %w", quotedDatabase, err)
	}
	return nil
}
