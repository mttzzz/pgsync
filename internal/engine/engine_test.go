package engine_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
	"github.com/mttzzz/pgsync/internal/models"
)

type stubEngine struct{}

func (stubEngine) Plan(context.Context, engine.PlanOptions) (*models.SyncPlan, error) {
	return &models.SyncPlan{}, nil
}

func (stubEngine) Execute(context.Context, *models.SyncPlan, engine.ProgressObserver) (*models.SyncResult, error) {
	return &models.SyncResult{}, nil
}

func TestEngineInterfaceSignature(t *testing.T) {
	t.Parallel()
	var syncEngine engine.Engine = stubEngine{}
	plan, err := syncEngine.Plan(context.Background(), engine.PlanOptions{})
	require.NoError(t, err)
	assert.NotNil(t, plan)

	result, err := syncEngine.Execute(context.Background(), plan, nil)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestPlanOptionsValidateRejectsNilReceiver(t *testing.T) {
	t.Parallel()
	var opts *engine.PlanOptions
	err := opts.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plan options are required")
}

func TestPlanOptionsValidateRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutate  func(*engine.PlanOptions)
		wantErr string
	}{
		{
			name: "missing remote endpoint",
			mutate: func(opts *engine.PlanOptions) {
				opts.Remote = config.Connection{Port: 5432, SSLMode: "require"}
			},
			wantErr: "remote host",
		},
		{
			name: "missing local endpoint",
			mutate: func(opts *engine.PlanOptions) {
				opts.Local = config.Connection{Port: 5432, SSLMode: "disable"}
			},
			wantErr: "local host",
		},
		{
			name: "bad remote port",
			mutate: func(opts *engine.PlanOptions) {
				opts.Remote.Port = 0
			},
			wantErr: "remote port",
		},
		{
			name: "bad remote ssl mode",
			mutate: func(opts *engine.PlanOptions) {
				opts.Remote.SSLMode = "bogus"
			},
			wantErr: "remote ssl_mode",
		},
		{
			name: "missing database",
			mutate: func(opts *engine.PlanOptions) {
				opts.Database = " \t "
			},
			wantErr: "database is required",
		},
		{
			name: "negative threads",
			mutate: func(opts *engine.PlanOptions) {
				opts.Threads = -1
			},
			wantErr: "threads must be >= 0",
		},
		{
			name: "unsupported mode",
			mutate: func(opts *engine.PlanOptions) {
				opts.Mode = engine.Mode("turbo")
			},
			wantErr: "engine mode must be auto|native|external",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := validPlanOptions()
			tt.mutate(&opts)

			err := opts.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestPlanOptionsValidateNormalizesThreadsDatabaseAndTables(t *testing.T) {
	t.Parallel()
	opts := validPlanOptions()
	opts.Threads = 0
	opts.Database = " app "
	opts.Tables = []string{
		" public.users ",
		"public.orders",
		"public.users",
		"",
		"\t",
		"analytics.events",
		" public.orders ",
	}

	require.NoError(t, opts.Validate())
	assert.Equal(t, runtime.NumCPU(), opts.Threads)
	assert.Equal(t, "app", opts.Database)
	assert.Equal(t, []string{"public.users", "public.orders", "analytics.events"}, opts.Tables)
}

func TestPlanOptionsValidateKeepsNilTablesNil(t *testing.T) {
	t.Parallel()
	opts := validPlanOptions()
	opts.Tables = nil

	require.NoError(t, opts.Validate())
	assert.Nil(t, opts.Tables)
}

func TestPlanOptionsValidateAcceptsSupportedModes(t *testing.T) {
	t.Parallel()
	for _, mode := range []engine.Mode{engine.ModeAuto, engine.ModeNative, engine.ModeExternal} {
		mode := mode
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			opts := validPlanOptions()
			opts.Mode = mode

			require.NoError(t, opts.Validate())
			assert.Equal(t, mode, opts.Mode)
		})
	}
}

func TestPlanOptionsValidatePreservesDryRunAndConnections(t *testing.T) {
	t.Parallel()
	opts := validPlanOptions()
	opts.DryRun = true
	remote := opts.Remote
	local := opts.Local

	require.NoError(t, opts.Validate())
	assert.True(t, opts.DryRun)
	assert.Equal(t, remote, opts.Remote)
	assert.Equal(t, local, opts.Local)
}

func validPlanOptions() engine.PlanOptions {
	return engine.PlanOptions{
		Remote:   validConnection("remote.example.com", "require"),
		Local:    validConnection("localhost", "disable"),
		Database: "app",
		Threads:  4,
		Mode:     engine.ModeNative,
	}
}

func validConnection(host string, sslMode string) config.Connection {
	return config.Connection{
		Host:     host,
		Port:     5432,
		User:     "postgres",
		Password: "secret",
		SSLMode:  sslMode,
	}
}
