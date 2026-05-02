package cli

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
)

func TestConfigPathAndShowHelpers(t *testing.T) {
	t.Parallel()
	store := &fakeConfigStore{path: "/tmp/config.toml", cfg: redactionConfig()}
	globals := &globalFlags{}
	path, err := configPath(globals, store)
	require.NoError(t, err)
	assert.Equal(t, "/tmp/config.toml", path)
	globals.ConfigPath = "custom.toml"
	path, err = configPath(globals, store)
	require.NoError(t, err)
	assert.Equal(t, "custom.toml", path)

	var text strings.Builder
	require.NoError(t, writeRedactedConfig(&text, "text", config.Redacted(store.cfg)))
	assert.Contains(t, text.String(), "xxxxx")
	assert.NotContains(t, text.String(), "secret")

	var json strings.Builder
	require.NoError(t, writeRedactedConfig(&json, "json", config.Redacted(store.cfg)))
	assert.Contains(t, json.String(), "config.show")
	assert.NotContains(t, json.String(), "secret")
}

func TestConfigCommandsAndTUIRunner(t *testing.T) {
	t.Parallel()
	storeCfg := redactionConfig()
	path := t.TempDir() + "/config.toml"
	require.NoError(t, config.Save(path, storeCfg))
	runner := &fakeTUIRunner{}
	out, _, err := executeRoot(t, App{TUIRunner: runner, EngineFactory: func(nilLogger *slog.Logger) (engine.Engine, error) { return &fakeEngine{}, nil }}, "--config", path, "config", "path")
	require.NoError(t, err)
	assert.Contains(t, out, path)

	out, _, err = executeRoot(t, App{TUIRunner: runner, EngineFactory: func(nilLogger *slog.Logger) (engine.Engine, error) { return &fakeEngine{}, nil }}, "--config", path, "config", "show")
	require.NoError(t, err)
	assert.Contains(t, out, "xxxxx")
	assert.NotContains(t, out, "secret")

	_, _, err = executeRoot(t, App{TUIRunner: runner, EngineFactory: func(nilLogger *slog.Logger) (engine.Engine, error) { return &fakeEngine{}, nil }}, "config")
	require.NoError(t, err)
	assert.Equal(t, TUIModeConfigEdit, runner.mode)

	_, _, err = executeRoot(t, App{TUIRunner: runner, EngineFactory: func(nilLogger *slog.Logger) (engine.Engine, error) { return &fakeEngine{}, nil }}, "--config", path, "config", "reset")
	require.NoError(t, err)
	assert.Equal(t, TUIModeConfigReset, runner.mode)

	_, _, err = executeRoot(t, App{TUIRunner: runner, EngineFactory: func(nilLogger *slog.Logger) (engine.Engine, error) { return &fakeEngine{}, nil }}, "tui")
	require.NoError(t, err)
	assert.Equal(t, TUIModeApp, runner.mode)

	_, _, err = executeRoot(t, App{EngineFactory: func(nilLogger *slog.Logger) (engine.Engine, error) { return &fakeEngine{}, nil }}, "tui")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotImplemented)
}

func TestConfigPathDefaultError(t *testing.T) {
	t.Parallel()
	_, err := configPath(&globalFlags{}, &fakeConfigStore{err: errors.New("no home")})
	require.Error(t, err)

	var out strings.Builder
	cmd := newConfigPathCommand(App{Out: &out}, &globalFlags{}, &fakeConfigStore{err: errors.New("no path")})
	err = cmd.Execute()
	require.Error(t, err)

	cmd = newConfigShowCommand(App{Out: &out}, &globalFlags{ConfigPath: "x"}, &fakeConfigStore{err: errors.New("load")})
	err = cmd.Execute()
	require.Error(t, err)

	cmd = newConfigShowCommand(App{Out: &out}, &globalFlags{}, &fakeConfigStore{err: errors.New("no path")})
	err = cmd.Execute()
	require.Error(t, err)
}

func TestFileConfigStoreMethods(t *testing.T) {
	t.Parallel()
	path := t.TempDir() + "/config.toml"
	store := fileConfigStore{env: map[string]string{"HOME": t.TempDir(), "APPDATA": t.TempDir()}}
	cfg := redactionConfig()
	require.NoError(t, store.Save(path, cfg))
	got, err := store.Load(path)
	require.NoError(t, err)
	assert.Equal(t, cfg.Remote.Host, got.Remote.Host)
	defaultPath, err := store.DefaultPath()
	require.NoError(t, err)
	assert.Contains(t, defaultPath, "pgsync")
	require.NoError(t, store.Remove(path))
}

func TestRootNoArgsRunsTUIWhenInjected(t *testing.T) {
	t.Parallel()
	runner := &fakeTUIRunner{}
	_, _, err := executeRoot(t, App{TUIRunner: runner, EngineFactory: func(nilLogger *slog.Logger) (engine.Engine, error) { return &fakeEngine{}, nil }})
	require.NoError(t, err)
	assert.Equal(t, TUIModeApp, runner.mode)
}

func redactionConfig() config.Config {
	cfg := config.Defaults()
	cfg.Remote.Host = "prod"
	cfg.Remote.User = "u"
	cfg.Remote.Password = "secret"
	cfg.Remote.ProxyURL = "socks5://u:secret@h:1"
	cfg.Local.Host = "localhost"
	cfg.Local.User = "postgres"
	cfg.Local.Password = "secret"
	return cfg
}

type fakeConfigStore struct {
	path string
	cfg  config.Config
	err  error
}

func (f *fakeConfigStore) Load(path string) (config.Config, error)   { return f.cfg, f.err }
func (f *fakeConfigStore) Save(path string, cfg config.Config) error { f.cfg = cfg; return f.err }
func (f *fakeConfigStore) DefaultPath() (string, error)              { return f.path, f.err }
func (f *fakeConfigStore) Remove(path string) error                  { return f.err }

type fakeTUIRunner struct {
	mode TUIMode
	err  error
}

func (f *fakeTUIRunner) Run(ctx context.Context, mode TUIMode) error {
	f.mode = mode
	return f.err
}
