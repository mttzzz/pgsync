package cli

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/models"
)

type fakeEngine struct {
	planCalls    int
	executeCalls int
	lastOptions  engine.PlanOptions
	planErr      error
	executeErr   error
	plan         *models.SyncPlan
	result       *models.SyncResult
}

func (f *fakeEngine) Plan(_ context.Context, opts engine.PlanOptions) (*models.SyncPlan, error) {
	f.planCalls++
	f.lastOptions = opts
	if f.planErr != nil {
		return nil, f.planErr
	}
	if f.plan != nil {
		return f.plan, nil
	}
	return &models.SyncPlan{
		Database: opts.Database,
		Tables:   modelTables(opts.Tables),
		DryRun:   opts.DryRun,
		Threads:  opts.Threads,
		Engine:   string(opts.Mode),
		Remote:   opts.Remote,
		Local:    opts.Local,
	}, nil
}

func (f *fakeEngine) Execute(ctx context.Context, plan *models.SyncPlan, observer engine.ProgressObserver) (*models.SyncResult, error) {
	f.executeCalls++
	if observer != nil {
		observer.OnEvent(ctx, engine.Event{
			Name:     engine.EventSyncStart,
			Database: plan.Database,
			Tables:   len(plan.Tables),
			Engine:   plan.Engine,
		})
		observer.OnEvent(ctx, engine.Event{
			Name:        engine.EventTableCopyProgress,
			Table:       "public.users",
			Bytes:       7,
			Percent:     50,
			BytesPerSec: 90,
		})
	}
	if f.executeErr != nil {
		return nil, f.executeErr
	}
	if f.result != nil {
		return f.result, nil
	}
	return &models.SyncResult{Database: plan.Database, TablesCopied: len(plan.Tables), RowsCopied: 7, BytesCopied: 9}, nil
}

func TestSyncYesPlansAndExecutes(t *testing.T) {
	t.Parallel()
	fake := &fakeEngine{}
	out, _, err := executeRoot(t, appWithEngine(fake), "--config", writeTestConfig(t, testConfig()), "sync", "mydb", "--yes")
	require.NoError(t, err)
	assert.Equal(t, 1, fake.planCalls)
	assert.Equal(t, 1, fake.executeCalls)
	assert.Equal(t, "mydb", fake.lastOptions.Database)
	assert.Contains(t, out, "starting sync")
	assert.Contains(t, out, "table public.users copy_stream_bytes=7 pct_of_disk_est=50.0% bytes_per_sec=90")
	assert.Contains(t, out, "synced")
	assert.Contains(t, out, "database=mydb")
}

func TestSyncCanUseDefaultDatabaseWhenArgumentOmitted(t *testing.T) {
	t.Parallel()
	fake := &fakeEngine{}
	out, _, err := executeRoot(t, appWithEngine(fake), "--config", writeTestConfig(t, testConfig()), "sync", "--dry-run")
	require.NoError(t, err)
	assert.Equal(t, "fixture-db", fake.lastOptions.Database)
	assert.Contains(t, out, "database=fixture-db")
}

func TestSyncDryRunPrintsPlanWithoutConfirmation(t *testing.T) {
	t.Parallel()
	fake := &fakeEngine{}
	out, _, err := executeRoot(t, appWithEngine(fake), "--config", writeTestConfig(t, testConfig()), "sync", "mydb", "--dry-run")
	require.NoError(t, err)
	assert.Equal(t, 1, fake.planCalls)
	assert.Zero(t, fake.executeCalls)
	assert.Contains(t, out, "database=mydb")
	assert.NotContains(t, out, "remote-pass")
	assert.NotContains(t, out, "local-pass")
}

func TestSyncRequiresConfirmation(t *testing.T) {
	t.Parallel()
	fake := &fakeEngine{}
	_, _, err := executeRoot(t, appWithEngine(fake), "--config", writeTestConfig(t, testConfig()), "sync", "mydb")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConfirmationRequired)
	assert.Equal(t, 1, fake.planCalls)
	assert.Zero(t, fake.executeCalls)
}

func TestSyncTablesAndGlobalFlagsOverrideConfig(t *testing.T) {
	t.Parallel()
	fake := &fakeEngine{}
	_, _, err := executeRoot(t, appWithEngine(fake),
		"--config", writeTestConfig(t, testConfig()),
		"--threads", "7",
		"--engine", "auto",
		"--use-system-pgtools=false",
		"--output", "json",
		"--quiet",
		"--verbose",
		"--no-color",
		"sync", "mydb",
		"--tables", "users,orders",
		"--dry-run",
		"--analyze",
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"users", "orders"}, fake.lastOptions.Tables)
	assert.Equal(t, 7, fake.lastOptions.Threads)
	assert.Equal(t, engine.ModeAuto, fake.lastOptions.Mode)
	assert.False(t, fake.lastOptions.UseSystemPgtools)
	assert.True(t, fake.lastOptions.DryRun)
	assert.True(t, fake.lastOptions.Analyze)
}

func TestSyncJSONObserver(t *testing.T) {
	t.Parallel()
	fake := &fakeEngine{}
	out, errOut, err := executeRoot(t, appWithEngine(fake),
		"--config", writeTestConfig(t, testConfig()),
		"--output", "json",
		"sync", "mydb",
		"--yes",
	)
	require.NoError(t, err)
	assert.Empty(t, errOut)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 2)
	start := decodeNDJSONLine(t, lines[0])
	assert.Equal(t, engine.EventSyncStart, start["event"])
	assert.Equal(t, "mydb", start["db"])
	progress := decodeNDJSONLine(t, lines[1])
	assert.Equal(t, engine.EventTableCopyProgress, progress["event"])
	assert.NotContains(t, out, "synced database")
}

func TestSyncQuietSuppressesObserver(t *testing.T) {
	t.Parallel()
	fake := &fakeEngine{}
	out, _, err := executeRoot(t, appWithEngine(fake),
		"--config", writeTestConfig(t, testConfig()),
		"--quiet",
		"sync", "mydb",
		"--yes",
	)
	require.NoError(t, err)
	assert.NotContains(t, out, "starting sync")
	assert.NotContains(t, out, "table public.users")
	assert.Contains(t, out, "synced")
	assert.Contains(t, out, "database=mydb")
}

func TestSyncReturnsResolveError(t *testing.T) {
	t.Parallel()
	fake := &fakeEngine{}
	_, _, err := executeRoot(t, appWithEngine(fake), "--config", "missing.toml", "sync", "mydb", "--dry-run")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load config")
	assert.Zero(t, fake.planCalls)
}

func TestSyncReturnsPlanOptionsError(t *testing.T) {
	t.Parallel()
	fake := &fakeEngine{}
	cfg := testConfig()
	cfg.Runtime.DefaultDatabase = ""
	cfg.Remote.Database = ""
	err := runSync(context.Background(), appWithEngine(fake), FlagOverrides{ConfigPath: writeTestConfig(t, cfg)}, SyncFlags{}, " ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database is required")
	assert.Zero(t, fake.planCalls)
}

func TestSyncReturnsFactoryErrorWithSecretsRedacted(t *testing.T) {
	t.Parallel()
	app := App{EngineFactory: func(*slog.Logger) (engine.Engine, error) {
		return nil, errors.New("factory failed with remote-pass and local-pass")
	}}
	_, _, err := executeRoot(t, app, "--config", writeTestConfig(t, testConfig()), "sync", "mydb", "--dry-run")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "remote-pass")
	assert.NotContains(t, err.Error(), "local-pass")
	assert.Contains(t, err.Error(), "******")
}

func TestSyncReturnsPlanErrorWithSecretsRedacted(t *testing.T) {
	t.Parallel()
	fake := &fakeEngine{planErr: errors.New("plan failed with remote-pass")}
	_, _, err := executeRoot(t, appWithEngine(fake), "--config", writeTestConfig(t, testConfig()), "sync", "mydb", "--dry-run")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "remote-pass")
	assert.Contains(t, err.Error(), "******")
}

func TestSyncReturnsExecuteErrorWithSecretsRedacted(t *testing.T) {
	t.Parallel()
	fake := &fakeEngine{executeErr: errors.New("execute failed with local-pass")}
	_, _, err := executeRoot(t, appWithEngine(fake), "--config", writeTestConfig(t, testConfig()), "sync", "mydb", "--yes")
	require.Error(t, err)
	assert.Equal(t, 1, fake.executeCalls)
	assert.NotContains(t, err.Error(), "local-pass")
	assert.Contains(t, err.Error(), "******")
}

func appWithEngine(fake *fakeEngine) App {
	return App{EngineFactory: func(*slog.Logger) (engine.Engine, error) {
		return fake, nil
	}}
}

func modelTables(names []string) []models.Table {
	tables := make([]models.Table, 0, len(names))
	for _, name := range names {
		tables = append(tables, models.Table{Schema: "public", Name: name})
	}
	return tables
}
