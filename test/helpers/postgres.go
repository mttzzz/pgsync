// Package helpers provides reusable test support for pgsync integration tests.
package helpers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/pgdb"
)

const (
	// PostgresImage is the deterministic PostgreSQL image used by Docker-backed
	// integration tests. The Phase-2 harness targets PostgreSQL 18 explicitly.
	PostgresImage = "postgres:18"

	postgresUser              = "postgres"
	postgresPassword          = "postgres"
	postgresContainerPort     = "5432/tcp"
	postgresCleanupTimeout    = 30 * time.Second
	postgresConnectionSSLMode = "disable"
)

// PostgresContainer describes a running PostgreSQL test container endpoint.
type PostgresContainer struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
}

// StartPostgres starts a PostgreSQL test container and registers test cleanup.
func StartPostgres(ctx context.Context, t testing.TB, database string) PostgresContainer {
	t.Helper()

	database = strings.TrimSpace(database)
	if database == "" {
		t.Fatal("postgres database name is required")
	}

	container, err := postgres.Run(ctx, PostgresImage,
		postgres.WithDatabase(database),
		postgres.WithUsername(postgresUser),
		postgres.WithPassword(postgresPassword),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container using %s: %v", PostgresImage, err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), postgresCleanupTimeout)
		defer cancel()
		if err := container.Terminate(cleanupCtx); err != nil {
			t.Errorf("terminate postgres container: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get postgres container host: %v", err)
	}
	mappedPort, err := container.MappedPort(ctx, postgresContainerPort)
	if err != nil {
		t.Fatalf("get postgres container mapped port: %v", err)
	}

	return PostgresContainer{
		Host:     host,
		Port:     int(mappedPort.Num()),
		User:     postgresUser,
		Password: postgresPassword,
		Database: database,
	}
}

// Config converts the running container endpoint into pgsync configuration.
func (p PostgresContainer) Config() config.Connection {
	return config.Connection{
		Host:     p.Host,
		Port:     p.Port,
		User:     p.User,
		Password: p.Password,
		Database: p.Database,
		SSLMode:  postgresConnectionSSLMode,
	}
}

// ExecSQLFile executes a SQL fixture file against the given PostgreSQL container.
func ExecSQLFile(ctx context.Context, t testing.TB, pg PostgresContainer, path string) {
	t.Helper()

	content, err := os.ReadFile(path) //nolint:gosec // Test fixtures are intentionally selected by the caller.
	if err != nil {
		t.Fatalf("read SQL fixture %q: %v", path, err)
	}

	conn := connectPostgres(ctx, t, pg)
	defer closePostgres(ctx, t, conn)

	if _, err := conn.Exec(ctx, string(content)); err != nil {
		t.Fatalf("execute SQL fixture %q: %v", path, err)
	}
}

func connectPostgres(ctx context.Context, t testing.TB, pg PostgresContainer) *pgx.Conn {
	t.Helper()

	connString, err := postgresConnectionString(pg)
	if err != nil {
		t.Fatalf("build postgres connection string: %v", err)
	}
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		t.Fatalf("connect to postgres %s: %v", pgdb.MaskConnString(connString), err)
	}
	return conn
}

func closePostgres(ctx context.Context, t testing.TB, conn *pgx.Conn) {
	t.Helper()

	if err := conn.Close(ctx); err != nil {
		t.Errorf("close postgres connection: %v", err)
	}
}

func postgresConnectionString(pg PostgresContainer) (string, error) {
	connString, err := pgdb.BuildConnString(pgdb.EndpointFromConfig(pg.Config(), ""))
	if err != nil {
		return "", fmt.Errorf("container config: %w", err)
	}
	return connString, nil
}
