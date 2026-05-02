package native

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/engine"
)

func TestPreparePlanOptionsSelectsNativeForAuto(t *testing.T) {
	t.Parallel()
	opts := nativeValidPlanOptions()
	opts.Mode = engine.ModeAuto

	err := preparePlanOptions(&opts, nil)

	require.NoError(t, err)
	assert.Equal(t, engine.ModeNative, opts.Mode)
}

func TestPreparePlanOptionsRejectsExternalAndInvalidOptions(t *testing.T) {
	t.Parallel()

	external := nativeValidPlanOptions()
	external.Mode = engine.ModeExternal
	err := preparePlanOptions(&external, nil)
	require.Error(t, err)
	assert.EqualError(t, err, phase2ExternalEngineMessage)

	invalid := nativeValidPlanOptions()
	invalid.Database = ""
	err = preparePlanOptions(&invalid, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database is required")
}

func TestPreparePlanOptionsAllowsNativeAndWarnsWhenSystemPgtoolsDisabled(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	opts := nativeValidPlanOptions()
	opts.Mode = engine.ModeNative
	opts.UseSystemPgtools = false

	err := preparePlanOptions(&opts, logger)

	require.NoError(t, err)
	assert.Equal(t, engine.ModeNative, opts.Mode)
	assert.Contains(t, logs.String(), "using system pg_dump/pg_restore")
}

func TestSelectPhase2ModeDefaultReturnsNilForAlreadyValidatedModes(t *testing.T) {
	t.Parallel()
	opts := nativeValidPlanOptions()
	opts.Mode = engine.Mode("future")

	err := selectPhase2Mode(&opts)

	require.NoError(t, err)
	assert.Equal(t, engine.Mode("future"), opts.Mode)
}

func TestWarnSystemPgtoolsReturnsWhenLoggerMissingOrSystemToolsEnabled(t *testing.T) {
	t.Parallel()
	warnSystemPgtools(false, nil)

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	warnSystemPgtools(true, logger)
	assert.Empty(t, logs.String())
}

func TestNewRequiresExplicitCoreDependencies(t *testing.T) {
	t.Parallel()
	_, err := New(Dependencies{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "connector")
	assert.Contains(t, err.Error(), "runner")
	assert.Contains(t, err.Error(), "locator")
	assert.Contains(t, err.Error(), "clock")
}

func TestNewAcceptsDependenciesAndDefaultsLoggerOnly(t *testing.T) {
	t.Parallel()
	deps := nativeTestDependencies(&nativeFakeConnector{})
	deps.Logger = nil

	eng, err := New(deps)

	require.NoError(t, err)
	assert.NotNil(t, eng)
	assert.NotNil(t, eng.deps.Logger)
	assert.NotNil(t, eng.stages.exportSnapshot)
}

func TestNewDefaultProvidesProductionDependencies(t *testing.T) {
	t.Parallel()
	eng, err := NewDefault(nil)

	require.NoError(t, err)
	assert.NotNil(t, eng.deps.Connector)
	assert.NotNil(t, eng.deps.Runner)
	assert.NotNil(t, eng.deps.Locator)
	assert.NotNil(t, eng.deps.Clock)
	assert.NotNil(t, eng.deps.Logger)
}

func TestNativeEngineNilReceiverErrors(t *testing.T) {
	t.Parallel()
	var eng *NativeEngine
	plan, err := eng.Plan(context.Background(), nativeValidPlanOptions())
	require.Error(t, err)
	assert.Nil(t, plan)
	assert.EqualError(t, err, "native engine is required")

	result, err := eng.Execute(context.Background(), nativeValidPlan(), nil)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.EqualError(t, err, "native engine is required")
}

type nativeFakeClock struct {
	now time.Time
}

func newNativeFakeClock() *nativeFakeClock {
	return &nativeFakeClock{now: time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)}
}

func (c *nativeFakeClock) Now() time.Time {
	current := c.now
	c.now = c.now.Add(time.Second)
	return current
}
