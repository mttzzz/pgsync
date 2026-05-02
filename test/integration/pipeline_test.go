//go:build integration

// Package integration contains Docker-backed integration tests for pgsync.
package integration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/jackc/pgx/v5"

	"github.com/mttzzz/pgsync/internal/cli"
	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/engine/native"
	"github.com/mttzzz/pgsync/internal/engine/pgtools"
	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/pgdb"
	"github.com/mttzzz/pgsync/test/helpers"
)

const tinyDatabaseName = "tiny"

var tinySyncTables = []string{
	"public.users",
	"public.orders",
	"public.order_items",
}

func TestNativePipelineSyncsTinyFixture(t *testing.T) {
	skipIfSystemPgDumpUnavailable(t)
	ctx, cancel := integrationContext(t)
	defer cancel()

	source, target := startSyncContainers(ctx, t)
	loadTinyFixture(ctx, t, source)

	eng := newNativeEngine(t)
	plan, err := eng.Plan(ctx, syncPlanOptions(source, target, nil, false))
	if err != nil {
		t.Fatalf("plan native sync: %v", err)
	}
	if len(plan.Tables) != len(tinySyncTables) {
		t.Fatalf("expected %d planned tables, got %d: %v", len(tinySyncTables), len(plan.Tables), tableNames(plan.Tables))
	}

	recorder := &recordingObserver{}
	result, err := eng.Execute(ctx, plan, recorder)
	if err != nil {
		t.Fatalf("execute native sync: %v", err)
	}
	assertSuccessfulResult(t, result, len(tinySyncTables))
	assertEventsContainInOrder(t, eventNames(recorder.Events()), []string{
		engine.EventSyncStart,
		engine.EventTableCopyStart,
		engine.EventTableCopyDone,
		engine.EventSyncDone,
	})

	helpers.AssertTableRowCountsEqual(ctx, t, source, target, tinySyncTables)
	helpers.AssertTableChecksumsEqual(ctx, t, source, target, tinySyncTables)
	helpers.AssertIndexExists(ctx, t, target, "users_name_idx")
	helpers.AssertIndexExists(ctx, t, target, "order_items_order_sku_idx")
	helpers.AssertFKExists(ctx, t, target, "orders_user_id_fkey")
	helpers.AssertFKExists(ctx, t, target, "order_items_order_id_fkey")
	helpers.AssertSequencesUsable(ctx, t, target, tinySyncTables)
}

func integrationContext(t testing.TB) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), harnessTimeout)
}

func skipIfSystemPgDumpUnavailable(t testing.TB) {
	t.Helper()

	bin := pgtools.BinDump()
	path, err := exec.LookPath(bin)
	if err != nil {
		t.Skipf("skipping real native sync: %s is required but was not found in PATH: %v", bin, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version") //nolint:gosec // Test validates the discovered pg_dump before using it.
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("skipping real native sync: %s --version failed: %v; output: %s", path, err, strings.TrimSpace(string(output)))
	}
}

func startSyncContainers(ctx context.Context, t testing.TB) (helpers.PostgresContainer, helpers.PostgresContainer) {
	t.Helper()
	source := helpers.StartPostgres(ctx, t, tinyDatabaseName)
	target := helpers.StartPostgres(ctx, t, tinyDatabaseName)
	return source, target
}

func loadTinyFixture(ctx context.Context, t testing.TB, pg helpers.PostgresContainer) {
	t.Helper()
	helpers.ExecSQLFile(ctx, t, pg, fixturePath("tiny.sql"))
}

func loadPartialFixture(ctx context.Context, t testing.TB, pg helpers.PostgresContainer) {
	t.Helper()
	helpers.ExecSQLFile(ctx, t, pg, fixturePath("partial.sql"))
}

func newNativeEngine(t testing.TB) *native.NativeEngine {
	t.Helper()
	eng, err := native.NewDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})))
	if err != nil {
		t.Fatalf("create native engine: %v", err)
	}
	return eng
}

func syncPlanOptions(
	source helpers.PostgresContainer,
	target helpers.PostgresContainer,
	tables []string,
	dryRun bool,
) engine.PlanOptions {
	return engine.PlanOptions{
		Remote:           source.Config(),
		Local:            syncLocalConfig(target),
		Database:         tinyDatabaseName,
		Tables:           tables,
		Threads:          2,
		Mode:             engine.ModeNative,
		UseSystemPgtools: true,
		DryRun:           dryRun,
		Yes:              true,
	}
}

func syncLocalConfig(target helpers.PostgresContainer) config.Connection {
	cfg := target.Config()
	cfg.Database = ""
	return cfg
}

func assertSuccessfulResult(t testing.TB, result *models.SyncResult, expectedTables int) {
	t.Helper()
	if result == nil {
		t.Fatal("expected sync result")
		return
	}
	if result.Err != nil {
		t.Fatalf("expected result without error, got %v", result.Err)
	}
	if result.TablesCopied != expectedTables {
		t.Fatalf("expected %d copied tables, got %d", expectedTables, result.TablesCopied)
	}
	assertStageDurations(t, result.Stages, []string{
		"snapshot",
		"dump-pre-data",
		"reset-target",
		"connect-target",
		"apply-pre-data",
		"copy",
		"dump-post-data",
		"apply-post-data",
		"repair-sequences",
	})
}

func assertStageDurations(t testing.TB, stages map[string]time.Duration, expected []string) {
	t.Helper()
	for _, stage := range expected {
		duration, ok := stages[stage]
		if !ok {
			t.Fatalf("expected stage duration for %q; got stages %v", stage, stageNames(stages))
		}
		if duration < 0 {
			t.Fatalf("expected non-negative duration for %q, got %s", stage, duration)
		}
	}
}

func stageNames(stages map[string]time.Duration) []string {
	names := make([]string, 0, len(stages))
	for name := range stages {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func assertEventsContainInOrder(t testing.TB, got []string, want []string) {
	t.Helper()
	pos := 0
	for _, eventName := range got {
		if pos < len(want) && eventName == want[pos] {
			pos++
		}
	}
	if pos != len(want) {
		t.Fatalf("expected events to contain %v in order, got %v", want, got)
	}
}

func tableNames(tables []models.Table) []string {
	names := make([]string, 0, len(tables))
	for _, table := range tables {
		names = append(names, table.Schema+"."+table.Name)
	}
	return names
}

func eventNames(events []engine.Event) []string {
	names := make([]string, 0, len(events))
	for _, event := range events {
		names = append(names, event.Name)
	}
	return names
}

type recordingObserver struct {
	mu       sync.Mutex
	events   []engine.Event
	cancel   context.CancelFunc
	cancelOn string
	canceled bool
}

func (o *recordingObserver) OnEvent(_ context.Context, event engine.Event) {
	var cancel context.CancelFunc
	o.mu.Lock()
	o.events = append(o.events, event)
	if !o.canceled && o.cancel != nil && event.Name == o.cancelOn {
		o.canceled = true
		cancel = o.cancel
	}
	o.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (o *recordingObserver) Events() []engine.Event {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]engine.Event, len(o.events))
	copy(out, o.events)
	return out
}

func runCLI(ctx context.Context, t testing.TB, args ...string) (string, string, error) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := cli.NewRootCommand(cli.App{
		Out: &stdout,
		Err: &stderr,
		In:  strings.NewReader(""),
	})
	cmd.SetContext(ctx)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func writeCLIConfig(t testing.TB, source helpers.PostgresContainer, target helpers.PostgresContainer) string {
	t.Helper()
	cfg := config.Config{
		Remote: source.Config(),
		Local:  syncLocalConfig(target),
		Runtime: config.Runtime{
			Threads:          2,
			Engine:           string(engine.ModeNative),
			UseSystemPgtools: true,
			DefaultDatabase:  tinyDatabaseName,
		},
		Logging: config.Logging{Level: "info", Format: "text"},
	}

	var encoded strings.Builder
	if err := toml.NewEncoder(&encoded).Encode(cfg); err != nil {
		t.Fatalf("encode CLI config: %v", err)
	}
	path := t.TempDir() + string(os.PathSeparator) + "pgsync.toml"
	if err := os.WriteFile(path, []byte(encoded.String()), 0o600); err != nil {
		t.Fatalf("write CLI config: %v", err)
	}
	return path
}

func decodeNDJSON(t testing.TB, output string) []map[string]any {
	t.Helper()
	scanner := bufio.NewScanner(strings.NewReader(strings.TrimSpace(output)))
	records := make([]map[string]any, 0)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("decode NDJSON line %q: %v", string(line), err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan NDJSON output: %v", err)
	}
	if len(records) == 0 {
		t.Fatalf("expected NDJSON output, got %q", output)
	}
	return records
}

func ndjsonEventNames(records []map[string]any) []string {
	names := make([]string, 0, len(records))
	for _, record := range records {
		if name, ok := record["event"].(string); ok {
			names = append(names, name)
		}
	}
	return names
}

func openContainerConn(ctx context.Context, t testing.TB, pg helpers.PostgresContainer) *pgx.Conn {
	t.Helper()
	connString, err := pgdb.BuildConnString(pgdb.EndpointFromConfig(pg.Config(), ""))
	if err != nil {
		t.Fatalf("build test connection string: %v", err)
	}
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		t.Fatalf("connect test postgres %s: %v", pgdb.MaskConnString(connString), err)
	}
	return conn
}

func execSQL(ctx context.Context, t testing.TB, pg helpers.PostgresContainer, sql string) {
	t.Helper()
	conn := openContainerConn(ctx, t, pg)
	defer closeConn(ctx, t, conn)
	if _, err := conn.Exec(ctx, sql); err != nil {
		t.Fatalf("execute SQL: %v", err)
	}
}

func closeConn(ctx context.Context, t testing.TB, conn *pgx.Conn) {
	t.Helper()
	if err := conn.Close(ctx); err != nil {
		t.Errorf("close test postgres connection: %v", err)
	}
}

func auditLogRowCountIfExists(ctx context.Context, t testing.TB, pg helpers.PostgresContainer) (int64, bool) {
	t.Helper()
	conn := openContainerConn(ctx, t, pg)
	defer closeConn(ctx, t, conn)

	var exists bool
	if err := conn.QueryRow(ctx, "SELECT to_regclass('public.audit_log') IS NOT NULL").Scan(&exists); err != nil {
		t.Fatalf("check audit_log existence: %v", err)
	}
	if !exists {
		return 0, false
	}

	var count int64
	if err := conn.QueryRow(ctx, "SELECT count(*)::bigint FROM public.audit_log").Scan(&count); err != nil {
		t.Fatalf("count audit_log rows: %v", err)
	}
	return count, true
}

func sentinelValue(ctx context.Context, t testing.TB, pg helpers.PostgresContainer) string {
	t.Helper()
	conn := openContainerConn(ctx, t, pg)
	defer closeConn(ctx, t, conn)

	var value string
	if err := conn.QueryRow(ctx, "SELECT value FROM public.dry_run_sentinel WHERE id = 1").Scan(&value); err != nil {
		t.Fatalf("query dry-run sentinel: %v", err)
	}
	return value
}

func containsTable(tables []models.Table, name string) bool {
	for _, table := range tables {
		if table.Schema+"."+table.Name == name {
			return true
		}
	}
	return false
}

func formatTables(tables []models.Table) string {
	return fmt.Sprintf("%v", tableNames(tables))
}
