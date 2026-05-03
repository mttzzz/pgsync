package native

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/pgdb"
)

func TestPlanBuildsTopoSortedClosureAndSequences(t *testing.T) {
	t.Parallel()
	conn := nativeCatalogConn(nativeCatalogData{
		tables: []models.Table{
			nativeTable("orders", 300, 30),
			nativeTable("users", 200, 20),
			nativeTable("orgs", 100, 10),
			nativeTable("audit", 50, 5),
		},
		deps: []models.FKDep{
			nativeDep("orders", "users"),
			nativeDep("users", "orgs"),
		},
		sequences: []models.Sequence{
			nativeSequence("users_id_seq", "users"),
			nativeSequence("orders_id_seq", "orders"),
			nativeSequence("audit_id_seq", "audit"),
		},
	})
	connector := &nativeFakeConnector{conns: []pgdb.CopyConn{conn}}
	eng := nativeTestEngine(t, connector)
	opts := nativeValidPlanOptions()
	opts.Mode = engine.ModeAuto
	opts.Tables = []string{"orders"}
	opts.DryRun = true
	opts.Threads = 3

	plan, err := eng.Plan(context.Background(), opts)

	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Equal(t, "appdb", plan.Database)
	assert.Equal(t, []string{"orgs", "users", "orders"}, nativeTableNames(plan.Tables))
	assert.Equal(t, []string{"users_id_seq", "orders_id_seq"}, nativeSequenceNames(plan.Sequences))
	assert.True(t, plan.DryRun)
	assert.Equal(t, 3, plan.Threads)
	assert.Equal(t, string(engine.ModeNative), plan.Engine)
	assert.Equal(t, opts.Remote, plan.Remote)
	assert.Equal(t, opts.Local, plan.Local)
	require.Len(t, connector.calls, 1)
	assert.Equal(t, "appdb", connector.calls[0].Database)
	assert.Equal(t, "remote.example.com", connector.calls[0].Host)
	assert.Equal(t, 1, conn.closeCount)
}

func TestPlanRejectsInvalidOptionsAndConnectErrors(t *testing.T) {
	t.Parallel()
	eng := nativeTestEngine(t, &nativeFakeConnector{err: errors.New("dial failed")})

	invalid := nativeValidPlanOptions()
	invalid.Database = ""
	plan, err := eng.Plan(context.Background(), invalid)
	require.Error(t, err)
	assert.Nil(t, plan)
	assert.Contains(t, err.Error(), "database is required")

	plan, err = eng.Plan(context.Background(), nativeValidPlanOptions())
	require.Error(t, err)
	assert.Nil(t, plan)
	assert.Contains(t, err.Error(), "connect remote database")
}

func TestPlanClosesRemoteConnectionOnCatalogAndGraphErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		conn    *nativeFakeConn
		tables  []string
		wantErr string
	}{
		{
			name:    "tables query",
			conn:    &nativeFakeConn{queryResults: []nativeQueryResult{{err: errors.New("tables failed")}}},
			wantErr: "list tables",
		},
		{
			name: "foreign keys query",
			conn: &nativeFakeConn{queryResults: []nativeQueryResult{
				{rows: nativeTableRows([]models.Table{nativeTable("users", 1, 1)})},
				{err: errors.New("fk failed")},
			}},
			wantErr: "list foreign keys",
		},
		{
			name: "sequences query",
			conn: &nativeFakeConn{queryResults: []nativeQueryResult{
				{rows: nativeTableRows([]models.Table{nativeTable("users", 1, 1)})},
				{rows: nativeDepRows(nil)},
				{err: errors.New("seq failed")},
			}},
			wantErr: "list sequences",
		},
		{
			name: "requested missing table",
			conn: nativeCatalogConn(nativeCatalogData{
				tables: []models.Table{nativeTable("users", 1, 1)},
			}),
			tables:  []string{"missing"},
			wantErr: "table not found",
		},
		{
			name: "topological cycle",
			conn: nativeCatalogConn(nativeCatalogData{
				tables: []models.Table{nativeTable("a", 1, 1), nativeTable("b", 1, 1)},
				deps:   []models.FKDep{nativeDep("a", "b"), nativeDep("b", "a")},
			}),
			wantErr: "FK cycle",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eng := nativeTestEngine(t, &nativeFakeConnector{conns: []pgdb.CopyConn{tt.conn}})
			opts := nativeValidPlanOptions()
			opts.Tables = tt.tables

			plan, err := eng.Plan(context.Background(), opts)

			require.Error(t, err)
			assert.Nil(t, plan)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Equal(t, 1, tt.conn.closeCount)
		})
	}
}

func TestPlanReturnsCloseErrors(t *testing.T) {
	t.Parallel()
	closeErr := errors.New("close failed")
	conn := nativeCatalogConn(nativeCatalogData{tables: []models.Table{nativeTable("users", 1, 1)}})
	conn.closeErr = closeErr
	eng := nativeTestEngine(t, &nativeFakeConnector{conns: []pgdb.CopyConn{conn}})

	plan, err := eng.Plan(context.Background(), nativeValidPlanOptions())

	require.Error(t, err)
	assert.NotNil(t, plan)
	assert.ErrorIs(t, err, closeErr)
}

func TestSequencesForTablesHandlesEmptyInputs(t *testing.T) {
	t.Parallel()
	assert.Nil(t, sequencesForTables(nil, []models.Table{nativeTable("users", 1, 1)}))
	assert.Nil(t, sequencesForTables([]models.Sequence{nativeSequence("users_id_seq", "users")}, nil))
}

func TestExecuteDryRunSkipsDestructiveStages(t *testing.T) {
	t.Parallel()
	recorder := &nativeStageRecorder{}
	eng := nativeTestEngine(t, &nativeFakeConnector{})
	installNativeStageFakes(eng, recorder)
	plan := nativeValidPlan()
	plan.DryRun = true

	result, err := eng.Execute(context.Background(), plan, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, recorder.calls)
	assert.Empty(t, eng.deps.Connector.(*nativeFakeConnector).calls)
	assert.Equal(t, "appdb", result.Database)
	assert.NoError(t, result.Err)
}

func TestExecuteHappyPathRunsStagesInOrder(t *testing.T) {
	t.Parallel()
	snapshotConn := &nativeFakeConn{name: "snapshot"}
	targetConn := &nativeFakeConn{name: "target"}
	copySourceConn := &nativeFakeConn{name: "copy-source"}
	copyTargetConn := &nativeFakeConn{name: "copy-target"}
	connector := &nativeFakeConnector{conns: []pgdb.CopyConn{snapshotConn, targetConn, copySourceConn, copyTargetConn}}
	recorder := &nativeStageRecorder{}
	eng := nativeTestEngine(t, connector)
	installNativeStageFakes(eng, recorder)
	observer := &nativeRecordingObserver{}

	result, err := eng.Execute(context.Background(), nativeValidPlan(), observer)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NoError(t, result.Err)
	assert.Equal(t, int64(42), result.BytesCopied)
	assert.Equal(t, int64(7), result.RowsCopied)
	assert.Equal(t, 2, result.TablesCopied)
	assert.Equal(t, []string{
		stageSnapshot,
		stageDumpPreData,
		stageCheckExtensions,
		stageResetTarget,
		stageApplyPreData,
		stageCopyTables,
		stageDumpPostData,
		stageApplyPostData,
		stageRepairSequences,
	}, recorder.calls)
	assert.Equal(t, []string{
		stageSnapshot,
		stageDumpPreData,
		stageCheckExtensions,
		stageResetTarget,
		stageConnectTarget,
		stageApplyPreData,
		stageCopyTables,
		stageDumpPostData,
		stageApplyPostData,
		stageRepairSequences,
	}, nativeDoneStages(observer.events))
	assert.Subset(t, nativeStageKeys(result.Stages), []string{stageSnapshot, stageDumpPreData, stageResetTarget})
	assert.Equal(t, 1, snapshotConn.closeCount)
	assert.Equal(t, []string{rollbackSQL}, snapshotConn.execSQL)
	assert.Equal(t, 1, targetConn.closeCount)
	assert.Equal(t, 1, copySourceConn.closeCount)
	assert.Equal(t, 1, copyTargetConn.closeCount)
	assert.Len(t, connector.calls, 4)
	assert.Equal(t, "appdb", connector.calls[0].Database)
	assert.Equal(t, "appdb", connector.calls[1].Database)
	assert.Equal(t, engine.EventSyncStart, observer.events[0].Name)
	assert.Equal(t, engine.EventSyncDone, observer.events[len(observer.events)-1].Name)
}

func TestExecuteStageFailuresEmitFailedEventAndCleanup(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		failStage     string
		connectFailAt int
		wantRetryable bool
	}{
		{name: stageSnapshot, failStage: stageSnapshot},
		{name: stageDumpPreData, failStage: stageDumpPreData},
		{name: stageCheckExtensions, failStage: stageCheckExtensions},
		{name: stageResetTarget, failStage: stageResetTarget},
		{name: stageConnectTarget, connectFailAt: 2},
		{name: stageApplyPreData, failStage: stageApplyPreData},
		{name: stageCopyTables, failStage: stageCopyTables, wantRetryable: true},
		{name: stageDumpPostData, failStage: stageDumpPostData},
		{name: stageApplyPostData, failStage: stageApplyPostData},
		{name: stageRepairSequences, failStage: stageRepairSequences},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			snapshotConn := &nativeFakeConn{name: "snapshot"}
			targetConn := &nativeFakeConn{name: "target"}
			connector := &nativeFakeConnector{
				conns:      []pgdb.CopyConn{snapshotConn, targetConn},
				failCall:   tt.connectFailAt,
				connectErr: nativeSecretError(),
			}
			recorder := &nativeStageRecorder{failStage: tt.failStage}
			eng := nativeTestEngine(t, connector)
			installNativeStageFakes(eng, recorder)
			observer := &nativeRecordingObserver{}

			result, err := eng.Execute(context.Background(), nativeValidPlan(), observer)

			require.Error(t, err)
			require.NotNil(t, result)
			assert.ErrorIs(t, result.Err, err)
			assert.Equal(t, engine.EventSyncFailed, observer.events[len(observer.events)-1].Name)
			assert.NotContains(t, observer.events[len(observer.events)-1].Error, "remotepass")
			assert.NotContains(t, observer.events[len(observer.events)-1].Error, "localpass")
			if tt.wantRetryable {
				assert.Contains(t, err.Error(), "retry sync to rebuild target")
				assert.Equal(t, int64(5), result.BytesCopied)
			}
			if tt.name != stageSnapshot || connector.failCall == 0 {
				assert.Equal(t, 1, snapshotConn.closeCount)
			}
			if nativeStageReached(observer.events, stageConnectTarget) && tt.name != stageConnectTarget {
				assert.Equal(t, 1, targetConn.closeCount)
			}
		})
	}
}

func TestExecuteSnapshotConnectFailure(t *testing.T) {
	t.Parallel()
	connector := &nativeFakeConnector{failCall: 1, connectErr: nativeSecretError()}
	eng := nativeTestEngine(t, connector)
	installNativeStageFakes(eng, &nativeStageRecorder{})
	observer := &nativeRecordingObserver{}

	result, err := eng.Execute(context.Background(), nativeValidPlan(), observer)

	require.Error(t, err)
	require.NotNil(t, result)
	assert.Contains(t, err.Error(), "connect snapshot source")
	assert.Equal(t, engine.EventSyncFailed, observer.events[len(observer.events)-1].Name)
	assert.Len(t, connector.calls, 1)
}

func TestExecuteContextCancellationStopsBeforeLaterStages(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	snapshotConn := &nativeFakeConn{name: "snapshot"}
	connector := &nativeFakeConnector{conns: []pgdb.CopyConn{snapshotConn}}
	recorder := &nativeStageRecorder{cancelStage: stageResetTarget, cancel: cancel}
	eng := nativeTestEngine(t, connector)
	installNativeStageFakes(eng, recorder)
	observer := &nativeRecordingObserver{}

	result, err := eng.Execute(ctx, nativeValidPlan(), observer)

	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, result)
	assert.Equal(t, engine.EventSyncFailed, observer.events[len(observer.events)-1].Name)
	assert.False(t, nativeStageReached(observer.events, stageConnectTarget))
	assert.Equal(t, []string{stageSnapshot, stageDumpPreData, stageCheckExtensions, stageResetTarget}, recorder.calls)
	assert.Equal(t, 1, snapshotConn.closeCount)
}

func TestExecuteValidatesPlan(t *testing.T) {
	t.Parallel()
	eng := nativeTestEngine(t, &nativeFakeConnector{})

	result, err := eng.Execute(context.Background(), nil, nil)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "sync plan is required")

	result, err = eng.Execute(context.Background(), &models.SyncPlan{}, nil)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "database is required")

	plan := nativeValidPlan()
	plan.Engine = string(engine.ModeExternal)
	result, err = eng.Execute(context.Background(), plan, nil)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "must be native")
}

func nativeTestEngine(t *testing.T, connector *nativeFakeConnector) *NativeEngine {
	t.Helper()
	eng, err := New(nativeTestDependencies(connector))
	require.NoError(t, err)
	return eng
}

func nativeTestDependencies(connector *nativeFakeConnector) Dependencies {
	return Dependencies{
		Connector: connector,
		Runner:    &fakeCommandRunner{},
		Locator:   &fakePgtoolsLocator{dumpPath: "pg_dump"},
		Clock:     newNativeFakeClock(),
		Logger:    slog.Default(),
	}
}

func nativeValidPlanOptions() engine.PlanOptions {
	return engine.PlanOptions{
		Remote:           nativeConnection("remote.example.com", "remotepass", "require"),
		Local:            nativeConnection("localhost", "localpass", "disable"),
		Database:         "appdb",
		Threads:          2,
		Mode:             engine.ModeNative,
		UseSystemPgtools: true,
	}
}

func nativeValidPlan() *models.SyncPlan {
	opts := nativeValidPlanOptions()
	return &models.SyncPlan{
		Database: opts.Database,
		Tables: []models.Table{
			nativeTable("orgs", 100, 10),
			nativeTable("users", 200, 20),
		},
		Sequences: []models.Sequence{nativeSequence("users_id_seq", "users")},
		Threads:   opts.Threads,
		Engine:    string(engine.ModeNative),
		Remote:    opts.Remote,
		Local:     opts.Local,
	}
}

func nativeConnection(host string, password string, sslMode string) config.Connection {
	return config.Connection{
		Host:     host,
		Port:     5432,
		User:     "postgres",
		Password: password,
		SSLMode:  sslMode,
	}
}

func nativeTable(name string, size int64, rows int64) models.Table {
	return models.Table{Schema: "public", Name: name, SizeBytes: size, Rows: rows}
}

func nativeDep(from string, to string) models.FKDep {
	return models.FKDep{From: nativeTable(from, 0, 0), To: nativeTable(to, 0, 0)}
}

func nativeSequence(name string, table string) models.Sequence {
	return models.Sequence{Schema: "public", Name: name, TableSchema: "public", TableName: table, ColumnName: "id"}
}

type nativeCatalogData struct {
	tables    []models.Table
	deps      []models.FKDep
	sequences []models.Sequence
}

func nativeCatalogConn(data nativeCatalogData) *nativeFakeConn {
	return &nativeFakeConn{queryResults: []nativeQueryResult{
		{rows: nativeTableRows(data.tables)},
		{rows: nativeDepRows(data.deps)},
		{rows: nativeSequenceRows(data.sequences)},
	}}
}

func nativeTableRows(tables []models.Table) [][]any {
	rows := make([][]any, len(tables))
	for i, table := range tables {
		rows[i] = []any{table.Schema, table.Name, table.SizeBytes, table.Rows}
	}
	return rows
}

func nativeDepRows(deps []models.FKDep) [][]any {
	rows := make([][]any, len(deps))
	for i, dep := range deps {
		rows[i] = []any{dep.From.Schema, dep.From.Name, dep.To.Schema, dep.To.Name}
	}
	return rows
}

func nativeSequenceRows(sequences []models.Sequence) [][]any {
	rows := make([][]any, len(sequences))
	for i, sequence := range sequences {
		rows[i] = []any{sequence.Schema, sequence.Name, sequence.TableSchema, sequence.TableName, sequence.ColumnName}
	}
	return rows
}

func nativeTableNames(tables []models.Table) []string {
	names := make([]string, len(tables))
	for i, table := range tables {
		names[i] = table.Name
	}
	return names
}

func nativeSequenceNames(sequences []models.Sequence) []string {
	names := make([]string, len(sequences))
	for i, sequence := range sequences {
		names[i] = sequence.Name
	}
	return names
}

type nativeFakeConnector struct {
	calls      []pgdb.Endpoint
	conns      []pgdb.CopyConn
	err        error
	failCall   int
	connectErr error
}

func (c *nativeFakeConnector) Connect(_ context.Context, endpoint pgdb.Endpoint) (pgdb.CopyConn, error) {
	c.calls = append(c.calls, endpoint)
	if c.err != nil {
		return nil, c.err
	}
	if c.failCall == len(c.calls) {
		return nil, c.connectErr
	}
	if len(c.conns) == 0 {
		return &nativeFakeConn{name: fmt.Sprintf("conn-%d", len(c.calls))}, nil
	}
	conn := c.conns[0]
	c.conns = c.conns[1:]
	return conn, nil
}

type nativeQueryResult struct {
	rows    [][]any
	err     error
	rowsErr error
}

type nativeFakeConn struct {
	name         string
	queryResults []nativeQueryResult
	execSQL      []string
	closeCount   int
	closeErr     error
}

func (c *nativeFakeConn) Query(_ context.Context, _ string, _ ...any) (pgdb.Rows, error) {
	if len(c.queryResults) == 0 {
		return nil, errors.New("unexpected query")
	}
	result := c.queryResults[0]
	c.queryResults = c.queryResults[1:]
	if result.err != nil {
		return nil, result.err
	}
	return &nativeFakeRows{rows: result.rows, err: result.rowsErr}, nil
}

func (c *nativeFakeConn) QueryRow(_ context.Context, _ string, _ ...any) pgdb.Row {
	return nativeFakeRow{err: errors.New("unexpected query row")}
}

func (c *nativeFakeConn) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	c.execSQL = append(c.execSQL, sql)
	return pgconn.CommandTag{}, nil
}

func (c *nativeFakeConn) Close(_ context.Context) error {
	c.closeCount++
	return c.closeErr
}

func (c *nativeFakeConn) CopyTo(_ context.Context, _ io.Writer, _ string) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unexpected copy to")
}

func (c *nativeFakeConn) CopyFrom(_ context.Context, _ io.Reader, _ string) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unexpected copy from")
}

func (c *nativeFakeConn) ExecMulti(_ context.Context, _ string) error {
	return errors.New("unexpected exec multi")
}

type nativeFakeRows struct {
	rows [][]any
	next int
	err  error
}

func (r *nativeFakeRows) Close() {}

func (r *nativeFakeRows) Err() error { return r.err }

func (r *nativeFakeRows) Next() bool {
	if r.next >= len(r.rows) {
		return false
	}
	r.next++
	return true
}

func (r *nativeFakeRows) Scan(dest ...any) error {
	if r.next == 0 || r.next > len(r.rows) {
		return errors.New("scan without current row")
	}
	row := r.rows[r.next-1]
	if len(row) != len(dest) {
		return fmt.Errorf("scan got %d destinations for %d values", len(dest), len(row))
	}
	for i, value := range row {
		if err := nativeAssign(dest[i], value); err != nil {
			return err
		}
	}
	return nil
}

func nativeAssign(dest any, value any) error {
	switch ptr := dest.(type) {
	case *string:
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("value %T is not string", value)
		}
		*ptr = text
	case *int64:
		number, ok := value.(int64)
		if !ok {
			return fmt.Errorf("value %T is not int64", value)
		}
		*ptr = number
	default:
		return fmt.Errorf("unsupported destination %T", dest)
	}
	return nil
}

type nativeFakeRow struct {
	err error
}

func (r nativeFakeRow) Scan(_ ...any) error { return r.err }

type nativeStageRecorder struct {
	calls       []string
	failStage   string
	cancelStage string
	cancel      context.CancelFunc
}

func installNativeStageFakes(eng *NativeEngine, recorder *nativeStageRecorder) {
	eng.stages = nativeStages{
		exportSnapshot:  recorder.exportSnapshot,
		dumpSchema:      recorder.dumpSchema,
		checkExtensions: recorder.checkExtensions,
		resetTarget:     recorder.resetTarget,
		applySQL:        recorder.applySQL,
		copyTables:      recorder.copyTables,
		repairSequences: recorder.repairSequences,
	}
}

func (r *nativeStageRecorder) exportSnapshot(_ context.Context, conn pgdb.CopyConn) (*Snapshot, error) {
	r.record(stageSnapshot)
	if r.shouldFail(stageSnapshot) {
		return nil, nativeSecretError()
	}
	return &Snapshot{ID: "snapshot-1", Conn: conn}, nil
}

func (r *nativeStageRecorder) dumpSchema(_ context.Context, _ pgdb.Endpoint, section SchemaSection) (string, error) {
	stage := schemaDumpStage(section)
	r.record(stage)
	if r.shouldFail(stage) {
		return "", nativeSecretError()
	}
	return string(section) + " SQL", nil
}

func (r *nativeStageRecorder) checkExtensions(_ context.Context, _ config.Connection, _ string) error {
	r.record(stageCheckExtensions)
	return r.stageErr(stageCheckExtensions)
}

func (r *nativeStageRecorder) resetTarget(_ context.Context, _ config.Connection, _ string) error {
	r.record(stageResetTarget)
	return r.stageErr(stageResetTarget)
}

func (r *nativeStageRecorder) applySQL(_ context.Context, _ pgdb.CopyConn, section SchemaSection, _ string) error {
	stage := schemaApplyStage(section)
	r.record(stage)
	return r.stageErr(stage)
}

func (r *nativeStageRecorder) copyTables(ctx context.Context, opts CopyTablesOptions) (*models.SyncResult, error) {
	r.record(stageCopyTables)
	if err := closeFactoryConns(ctx, opts); err != nil {
		return nil, err
	}
	result := &models.SyncResult{BytesCopied: 42, RowsCopied: 7, TablesCopied: len(opts.Tables)}
	if r.shouldFail(stageCopyTables) {
		result.BytesCopied = 5
		return result, nativeSecretError()
	}
	return result, nil
}

func (r *nativeStageRecorder) repairSequences(_ context.Context, _ pgdb.CopyConn, _ []models.Sequence) error {
	r.record(stageRepairSequences)
	return r.stageErr(stageRepairSequences)
}

func schemaDumpStage(section SchemaSection) string {
	if section == SchemaPostData {
		return stageDumpPostData
	}
	return stageDumpPreData
}

func schemaApplyStage(section SchemaSection) string {
	if section == SchemaPostData {
		return stageApplyPostData
	}
	return stageApplyPreData
}

func closeFactoryConns(ctx context.Context, opts CopyTablesOptions) error {
	source, sourceErr := opts.SourceFactory(ctx)
	if sourceErr != nil {
		return sourceErr
	}
	target, targetErr := opts.TargetFactory(ctx)
	if targetErr != nil {
		return targetErr
	}
	_ = source.Close(context.WithoutCancel(ctx))
	_ = target.Close(context.WithoutCancel(ctx))
	return nil
}

func (r *nativeStageRecorder) stageErr(stage string) error {
	if r.shouldFail(stage) {
		return nativeSecretError()
	}
	return nil
}

func (r *nativeStageRecorder) record(stage string) {
	r.calls = append(r.calls, stage)
	if r.cancelStage == stage && r.cancel != nil {
		r.cancel()
	}
}

func (r *nativeStageRecorder) shouldFail(stage string) bool {
	return r.failStage == stage
}

func nativeSecretError() error {
	return errors.New("boom remotepass localpass password=localpass")
}

type nativeRecordingObserver struct {
	events []engine.Event
}

func (o *nativeRecordingObserver) OnEvent(_ context.Context, event engine.Event) {
	o.events = append(o.events, event)
}

func nativeDoneStages(events []engine.Event) []string {
	stages := make([]string, 0)
	for _, event := range events {
		if event.Stage != "sync" && event.Name == event.Stage+".done" {
			stages = append(stages, event.Stage)
		}
	}
	return stages
}

func nativeStageReached(events []engine.Event, stage string) bool {
	for _, event := range events {
		if event.Stage == stage {
			return true
		}
	}
	return false
}

func nativeStageKeys(stages map[string]time.Duration) []string {
	keys := make([]string, 0, len(stages))
	for key := range stages {
		keys = append(keys, key)
	}
	return keys
}
