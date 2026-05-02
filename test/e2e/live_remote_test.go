//go:build live

package e2e_test

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/pgdb"
	"github.com/mttzzz/pgsync/internal/pgschema"
)

func TestLiveRemoteDatabaseCycle(t *testing.T) {
	liveDatabaseName := strings.TrimSpace(os.Getenv("PGSYNC_LIVE_DATABASE"))
	if liveDatabaseName == "" {
		t.Skip("set PGSYNC_LIVE_DATABASE to run live remote database cycle")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	path, err := config.DefaultPath(envMapForLiveTest(os.Environ()))
	require.NoError(t, err)
	cfg, err := config.Load(path)
	require.NoError(t, err)
	require.NoError(t, config.Validate(cfg))

	connector := pgdb.NewConnector()
	databases, err := listLiveDatabases(ctx, connector, cfg)
	require.NoError(t, err)
	require.True(t, slices.ContainsFunc(databases, func(database models.Database) bool { return database.Name == liveDatabaseName }), "configured remote should contain %q; got %v", liveDatabaseName, databaseNames(databases))

	databaseConn, err := connector.Connect(ctx, pgdb.EndpointFromConfig(cfg.Remote, liveDatabaseName))
	require.NoError(t, err)
	defer func() { _ = databaseConn.Close(context.WithoutCancel(ctx)) }()

	tables, err := pgschema.NewService(databaseConn).ListTables(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, tables, "%s should have user tables", liveDatabaseName)
}

func listLiveDatabases(ctx context.Context, connector pgdb.Connector, cfg config.Config) ([]models.Database, error) {
	var lastErr error
	for _, database := range []string{cfg.Remote.Database, cfg.Runtime.DefaultDatabase, "defaultdb", ""} {
		conn, err := connector.Connect(ctx, pgdb.EndpointFromConfig(cfg.Remote, strings.TrimSpace(database)))
		if err != nil {
			lastErr = err
			continue
		}
		databases, listErr := pgschema.NewService(conn).ListDatabases(ctx)
		closeErr := conn.Close(context.WithoutCancel(ctx))
		if listErr != nil {
			lastErr = listErr
			continue
		}
		if closeErr != nil {
			lastErr = closeErr
			continue
		}
		return databases, nil
	}
	return nil, lastErr
}

func databaseNames(databases []models.Database) []string {
	names := make([]string, len(databases))
	for i, database := range databases {
		names[i] = database.Name
	}
	return names
}

func envMapForLiveTest(entries []string) map[string]string {
	env := make(map[string]string, len(entries))
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}
