package native

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	clockpkg "github.com/mttzzz/pgsync/internal/clock"
	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/engine/pgtools"
	"github.com/mttzzz/pgsync/internal/models"
	"github.com/mttzzz/pgsync/internal/pgdb"
	"github.com/mttzzz/pgsync/internal/pgschema"
	"github.com/mttzzz/pgsync/internal/runner"
)

const (
	stageSnapshot        = "snapshot"
	stageDumpPreData     = "dump-pre-data"
	stageResetTarget     = "reset-target"
	stageConnectTarget   = "connect-target"
	stageApplyPreData    = "apply-pre-data"
	stageCopyTables      = "copy"
	stageDumpPostData    = "dump-post-data"
	stageApplyPostData   = "apply-post-data"
	stageRepairSequences = "repair-sequences"
)

// Dependencies contains collaborators required by the native engine.
type Dependencies struct {
	Connector pgdb.Connector
	Runner    runner.CommandRunner
	Locator   pgtools.Locator
	Clock     clockpkg.Clock
	Logger    *slog.Logger
}

// NativeEngine plans and executes PostgreSQL syncs using native Go stages.
//
//revive:disable-next-line:exported -- name is required by the Phase 2 engine API.
type NativeEngine struct {
	deps   Dependencies
	stages nativeStages
}

type nativeStages struct {
	exportSnapshot  func(context.Context, pgdb.CopyConn) (*Snapshot, error)
	dumpSchema      func(context.Context, pgdb.Endpoint, SchemaSection) (string, error)
	resetTarget     func(context.Context, config.Connection, string) error
	applySQL        func(context.Context, pgdb.CopyConn, SchemaSection, string) error
	copyTables      func(context.Context, CopyTablesOptions) (*models.SyncResult, error)
	repairSequences func(context.Context, pgdb.CopyConn, []models.Sequence) error
}

// New creates a NativeEngine with explicit dependencies for tests and custom wiring.
func New(deps Dependencies) (*NativeEngine, error) {
	if err := validateDependencies(deps); err != nil {
		return nil, err
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	return &NativeEngine{deps: deps, stages: productionStages(deps)}, nil
}

// NewDefault creates a NativeEngine with production dependencies.
func NewDefault(logger *slog.Logger) (*NativeEngine, error) {
	if logger == nil {
		logger = slog.Default()
	}
	return New(Dependencies{
		Connector: pgdb.NewConnector(),
		Runner:    runner.NewExec(),
		Locator:   pgtools.NewSystemLocator(pgtools.ExecLooker{}),
		Clock:     clockpkg.NewSystem(),
		Logger:    logger,
	})
}

// Plan connects to the remote database, reads catalog metadata, and builds a sync plan.
func (e *NativeEngine) Plan(ctx context.Context, opts engine.PlanOptions) (plan *models.SyncPlan, err error) {
	if e == nil {
		return nil, errors.New("native engine is required")
	}
	if err := preparePlanOptions(&opts, e.deps.Logger); err != nil {
		return nil, err
	}

	endpoint := pgdb.EndpointFromConfig(opts.Remote, opts.Database)
	conn, err := e.deps.Connector.Connect(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("connect remote database %q: %w", opts.Database, err)
	}
	defer func() {
		if closeErr := conn.Close(context.WithoutCancel(ctx)); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close remote catalog connection: %w", closeErr))
		}
	}()

	catalog := pgschema.NewService(conn)
	tables, deps, sequences, err := listCatalog(ctx, catalog)
	if err != nil {
		return nil, err
	}
	selected, err := pgschema.FKClosure(tables, deps, opts.Tables)
	if err != nil {
		return nil, err
	}
	ordered, err := pgschema.TopoSort(selected, deps)
	if err != nil {
		return nil, err
	}

	return &models.SyncPlan{
		Database:  opts.Database,
		Tables:    ordered,
		Sequences: sequencesForTables(sequences, ordered),
		DryRun:    opts.DryRun,
		Threads:   opts.Threads,
		Engine:    string(engine.ModeNative),
		Remote:    opts.Remote,
		Local:     opts.Local,
	}, nil
}

// Execute runs a previously built sync plan.
func (e *NativeEngine) Execute(
	ctx context.Context,
	plan *models.SyncPlan,
	observer engine.ProgressObserver,
) (*models.SyncResult, error) {
	if e == nil {
		return nil, errors.New("native engine is required")
	}
	if err := validateExecutePlan(plan); err != nil {
		return nil, err
	}

	run := newExecutionRun(ctx, e, plan, observer)
	return run.execute()
}

func validateDependencies(deps Dependencies) error {
	missing := missingDependencies(deps)
	if len(missing) > 0 {
		return fmt.Errorf("native engine dependencies are required: %s", strings.Join(missing, ", "))
	}
	return nil
}

func missingDependencies(deps Dependencies) []string {
	missing := make([]string, 0, 4)
	if deps.Connector == nil {
		missing = append(missing, "connector")
	}
	if deps.Runner == nil {
		missing = append(missing, "runner")
	}
	if deps.Locator == nil {
		missing = append(missing, "locator")
	}
	if deps.Clock == nil {
		missing = append(missing, "clock")
	}
	return missing
}

func productionStages(deps Dependencies) nativeStages {
	dumper := &SchemaDumper{Runner: deps.Runner, Locator: deps.Locator}
	target := &TargetManager{Connector: deps.Connector}
	return nativeStages{
		exportSnapshot:  ExportSnapshot,
		dumpSchema:      dumper.Dump,
		resetTarget:     target.ResetDatabase,
		applySQL:        ApplySQL,
		copyTables:      CopyTables,
		repairSequences: RepairSequences,
	}
}

func listCatalog(ctx context.Context, catalog *pgschema.Service) ([]models.Table, []models.FKDep, []models.Sequence, error) {
	tables, err := catalog.ListTables(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	deps, err := catalog.ListFKDeps(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	sequences, err := catalog.ListSequences(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	return tables, deps, sequences, nil
}

func sequencesForTables(sequences []models.Sequence, tables []models.Table) []models.Sequence {
	if len(sequences) == 0 || len(tables) == 0 {
		return nil
	}
	selected := tableKeySet(tables)
	out := make([]models.Sequence, 0, len(sequences))
	for _, sequence := range sequences {
		if _, ok := selected[sequence.OwnedTable().QualifiedName()]; ok {
			out = append(out, sequence)
		}
	}
	return out
}

func tableKeySet(tables []models.Table) map[string]struct{} {
	selected := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		selected[table.QualifiedName()] = struct{}{}
	}
	return selected
}

func validateExecutePlan(plan *models.SyncPlan) error {
	if plan == nil {
		return errors.New("sync plan is required")
	}
	if plan.IsEmpty() {
		return errors.New("sync plan database is required")
	}
	if plan.Engine != "" && plan.Engine != string(engine.ModeNative) {
		return fmt.Errorf("sync plan engine must be native, got %q", plan.Engine)
	}
	return nil
}

type executionRun struct {
	ctx      context.Context
	engine   *NativeEngine
	plan     *models.SyncPlan
	observer engine.ProgressObserver
	result   *models.SyncResult
}

func newExecutionRun(
	ctx context.Context,
	nativeEngine *NativeEngine,
	plan *models.SyncPlan,
	observer engine.ProgressObserver,
) *executionRun {
	started := nativeEngine.deps.Clock.Now()
	return &executionRun{
		ctx:      ctx,
		engine:   nativeEngine,
		plan:     plan,
		observer: observer,
		result: &models.SyncResult{
			Database:  plan.Database,
			StartedAt: started,
			Stages:    make(map[string]time.Duration),
		},
	}
}

func (r *executionRun) execute() (*models.SyncResult, error) {
	r.emitSync(engine.EventSyncStart, "info", nil)
	if r.plan.DryRun {
		return r.finishSuccess()
	}

	err := r.executeStages()
	if err != nil {
		return r.finishFailure(err)
	}
	return r.finishSuccess()
}

func (r *executionRun) executeStages() (err error) {
	remote := pgdb.EndpointFromConfig(r.plan.Remote, r.plan.Database)
	local := pgdb.EndpointFromConfig(r.plan.Local, r.plan.Database)

	snapshot, err := r.openSnapshot(remote)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, snapshot.Close(context.WithoutCancel(r.ctx)))
	}()

	preDataSQL, err := r.dumpSchema(remote, SchemaPreData, stageDumpPreData)
	if err != nil {
		return err
	}
	if err := r.runVoidStage(stageResetTarget, func(ctx context.Context) error {
		return r.engine.stages.resetTarget(ctx, r.plan.Local, r.plan.Database)
	}); err != nil {
		return err
	}

	target, err := r.connectTarget(local)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, target.Close(context.WithoutCancel(r.ctx)))
	}()

	if err := r.applySQL(target, SchemaPreData, preDataSQL, stageApplyPreData); err != nil {
		return err
	}
	if err := r.copyTables(remote, local, snapshot.ID); err != nil {
		return err
	}
	postDataSQL, err := r.dumpSchema(remote, SchemaPostData, stageDumpPostData)
	if err != nil {
		return err
	}
	if err := r.applySQL(target, SchemaPostData, postDataSQL, stageApplyPostData); err != nil {
		return err
	}
	return r.runVoidStage(stageRepairSequences, func(ctx context.Context) error {
		return r.engine.stages.repairSequences(ctx, target, r.plan.Sequences)
	})
}

func (r *executionRun) openSnapshot(remote pgdb.Endpoint) (*Snapshot, error) {
	var snapshot *Snapshot
	var snapshotConn pgdb.CopyConn
	err := r.runVoidStage(stageSnapshot, func(ctx context.Context) error {
		conn, err := r.engine.deps.Connector.Connect(ctx, remote)
		if err != nil {
			return fmt.Errorf("connect snapshot source: %w", err)
		}
		snapshotConn = conn
		snapshot, err = r.engine.stages.exportSnapshot(ctx, conn)
		return err
	})
	if err != nil {
		if snapshot == nil && snapshotConn != nil {
			err = errors.Join(err, snapshotConn.Close(context.WithoutCancel(r.ctx)))
		}
		return nil, err
	}
	return snapshot, nil
}

func (r *executionRun) dumpSchema(remote pgdb.Endpoint, section SchemaSection, stage string) (string, error) {
	var sql string
	err := r.runVoidStage(stage, func(ctx context.Context) error {
		var err error
		sql, err = r.engine.stages.dumpSchema(ctx, remote, section)
		return err
	})
	return sql, err
}

func (r *executionRun) connectTarget(local pgdb.Endpoint) (pgdb.CopyConn, error) {
	var target pgdb.CopyConn
	err := r.runVoidStage(stageConnectTarget, func(ctx context.Context) error {
		var err error
		target, err = r.engine.deps.Connector.Connect(ctx, local)
		if err != nil {
			return fmt.Errorf("connect target database: %w", err)
		}
		return nil
	})
	return target, err
}

func (r *executionRun) applySQL(target pgdb.CopyConn, section SchemaSection, sql string, stage string) error {
	return r.runVoidStage(stage, func(ctx context.Context) error {
		return r.engine.stages.applySQL(ctx, target, section, sql)
	})
}

func (r *executionRun) copyTables(remote pgdb.Endpoint, local pgdb.Endpoint, snapshotID string) error {
	var copyResult *models.SyncResult
	err := r.runVoidStage(stageCopyTables, func(ctx context.Context) error {
		var err error
		copyResult, err = r.engine.stages.copyTables(ctx, CopyTablesOptions{
			Tables:        r.plan.Tables,
			SnapshotID:    snapshotID,
			Threads:       r.plan.Threads,
			SourceFactory: r.connectFactory(remote),
			TargetFactory: r.connectFactory(local),
			Observer:      r.observer,
			Clock:         r.engine.deps.Clock,
		})
		return err
	})
	if copyResult != nil {
		r.addCopyResult(copyResult)
	}
	if err != nil {
		return fmt.Errorf("copy table data failed after target reset; retry sync to rebuild target: %w", err)
	}
	return nil
}

func (r *executionRun) connectFactory(endpoint pgdb.Endpoint) func(context.Context) (pgdb.CopyConn, error) {
	return func(ctx context.Context) (pgdb.CopyConn, error) {
		return r.engine.deps.Connector.Connect(ctx, endpoint)
	}
}

func (r *executionRun) addCopyResult(copyResult *models.SyncResult) {
	r.result.BytesCopied += copyResult.BytesCopied
	r.result.RowsCopied += copyResult.RowsCopied
	r.result.TablesCopied += copyResult.TablesCopied
}

func (r *executionRun) runVoidStage(stage string, fn func(context.Context) error) error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	started := r.engine.deps.Clock.Now()
	r.emitStage(stage, stage+".start", "info", started, time.Time{}, nil)
	if err := fn(r.ctx); err != nil {
		r.emitStage(stage, stage+".failed", "error", started, r.engine.deps.Clock.Now(), err)
		return fmt.Errorf("%s: %w", stage, err)
	}
	r.emitStage(stage, stage+".done", "info", started, r.engine.deps.Clock.Now(), nil)
	return nil
}

func (r *executionRun) finishSuccess() (*models.SyncResult, error) {
	r.result.FinishedAt = r.engine.deps.Clock.Now()
	r.emitSync(engine.EventSyncDone, "info", nil)
	return r.result, nil
}

func (r *executionRun) finishFailure(err error) (*models.SyncResult, error) {
	r.result.Err = err
	r.result.FinishedAt = r.engine.deps.Clock.Now()
	r.emitSync(engine.EventSyncFailed, "error", err)
	return r.result, err
}

func (r *executionRun) emitStage(
	stage string,
	name string,
	level string,
	started time.Time,
	finished time.Time,
	err error,
) {
	if finished.IsZero() {
		finished = started
	}
	duration := finished.Sub(started)
	if strings.HasSuffix(name, ".done") {
		r.result.Stages[stage] = duration
	}
	r.emit(engine.Event{
		Time:     finished,
		Level:    level,
		Name:     name,
		Stage:    stage,
		Database: r.plan.Database,
		Engine:   string(engine.ModeNative),
		Tables:   len(r.plan.Tables),
		Duration: duration,
		Error:    r.redactError(err),
	})
}

func (r *executionRun) emitSync(name string, level string, err error) {
	r.emit(engine.Event{
		Time:     r.engine.deps.Clock.Now(),
		Level:    level,
		Name:     name,
		Stage:    "sync",
		Database: r.plan.Database,
		Engine:   string(engine.ModeNative),
		Tables:   len(r.plan.Tables),
		Duration: r.result.Duration(),
		Error:    r.redactError(err),
	})
}

func (r *executionRun) emit(event engine.Event) {
	if r.observer == nil {
		return
	}
	r.observer.OnEvent(r.ctx, event)
}

func (r *executionRun) redactError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	message = redactEndpointText(message, pgdb.EndpointFromConfig(r.plan.Remote, r.plan.Database), "")
	message = redactEndpointText(message, pgdb.EndpointFromConfig(r.plan.Local, r.plan.Database), "")
	return message
}
