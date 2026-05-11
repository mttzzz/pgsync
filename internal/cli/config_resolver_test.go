package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
)

func TestResolverMergesDefaultsFileEnvAndFlags(t *testing.T) {
	t.Parallel()
	useSystem := false
	path := writeTestConfig(t, testConfig())
	got, err := (Resolver{StorePath: path, Env: map[string]string{
		"PGSYNC_REMOTE_HOST": "env-remote",
		"PGSYNC_LOCAL_HOST":  "env-local",
		"PGSYNC_THREADS":     "6",
	}}).Resolve(context.Background(), FlagOverrides{
		Threads:          9,
		Engine:           "auto",
		UseSystemPgtools: &useSystem,
		Remote: config.Connection{
			Host:     "flag-remote",
			Port:     6543,
			User:     "flag-user",
			Password: "flag-token",
			Database: "flagdb",
			SSLMode:  "verify-full",
			ProxyURL: "https://proxy.example.com:8443",
		},
		Local: config.Connection{
			Host:     "flag-local",
			Port:     7654,
			User:     "flag-local-user",
			Password: "flag-local-token",
			Database: "flaglocaldb",
			SSLMode:  "disable",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "flag-remote", got.Remote.Host)
	assert.Equal(t, 6543, got.Remote.Port)
	assert.Equal(t, "flag-user", got.Remote.User)
	assert.Equal(t, "flag-token", got.Remote.Password)
	assert.Equal(t, "flagdb", got.Remote.Database)
	assert.Equal(t, "verify-full", got.Remote.SSLMode)
	assert.Equal(t, "https://proxy.example.com:8443", got.Remote.ProxyURL)
	assert.Equal(t, "flag-local", got.Local.Host)
	assert.Equal(t, 7654, got.Local.Port)
	assert.Equal(t, "flag-local-user", got.Local.User)
	assert.Equal(t, "flag-local-token", got.Local.Password)
	assert.Equal(t, "flaglocaldb", got.Local.Database)
	assert.Equal(t, "disable", got.Local.SSLMode)
	assert.Equal(t, 9, got.Runtime.Threads)
	assert.Equal(t, "auto", got.Runtime.Engine)
	assert.False(t, got.Runtime.UseSystemPgtools)
	assert.True(t, got.Runtime.ConcurrentIndexes)
	assert.Equal(t, "fixture-db", got.Runtime.DefaultDatabase)
	assert.Equal(t, "debug", got.Logging.Level)
	assert.Equal(t, "json", got.Logging.Format)
}

func TestResolveUsesProcessEnvironment(t *testing.T) {
	clearPGSyncEnv(t)
	path := writeTestConfig(t, testConfig())
	got, err := Resolve(context.Background(), FlagOverrides{ConfigPath: path})
	require.NoError(t, err)
	assert.Equal(t, "file-remote", got.Remote.Host)
}

func TestResolveLoadsDotEnvPostgresURL(t *testing.T) {
	clearPGSyncEnv(t)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	require.NoError(t, config.Save(cfgPath, testConfig()))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte(strings.Join([]string{
		strings.Join([]string{"POSTGRES_URL='postgresql://app:", "sec", "ret", "@dotenv.example.com:6543/dotenv_db?sslmode=disable'"}, ""),
	}, "\n")), 0o600))
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	got, err := Resolve(context.Background(), FlagOverrides{ConfigPath: cfgPath})
	require.NoError(t, err)
	assert.Equal(t, "file-remote", got.Remote.Host)
	assert.Equal(t, "dotenv.example.com", got.Local.Host)
	assert.Equal(t, "dotenv_db", got.Runtime.DefaultDatabase)
}

func TestResolverUsesProcessEnvironmentWhenEnvIsNil(t *testing.T) {
	clearPGSyncEnv(t)
	path := writeTestConfig(t, testConfig())
	got, err := (Resolver{StorePath: path}).Resolve(context.Background(), FlagOverrides{})
	require.NoError(t, err)
	assert.Equal(t, "file-local", got.Local.Host)
}

func TestResolverReturnsContextError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (Resolver{}).Resolve(ctx, FlagOverrides{})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestResolverSurfacesConfigLoadErrorWhenOverridesAreIncomplete(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "missing.toml")
	_, err := (Resolver{StorePath: missing, Env: map[string]string{}}).Resolve(context.Background(), FlagOverrides{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load config")
}

func TestResolverUsesPostgresURLForLocalAndDefaultDatabase(t *testing.T) {
	t.Parallel()
	path := writeTestConfig(t, testConfig())
	got, err := (Resolver{StorePath: path, Env: map[string]string{
		"POSTGRES_URL": strings.Join([]string{"postgres://app:", "sec", "ret", "@local.example.com:6543/url_db?sslmode=disable"}, ""),
	}}).Resolve(context.Background(), FlagOverrides{})
	require.NoError(t, err)
	assert.Equal(t, "file-remote", got.Remote.Host)
	assert.Equal(t, "local.example.com", got.Local.Host)
	assert.Equal(t, 6543, got.Local.Port)
	assert.Equal(t, "app", got.Local.User)
	assert.Equal(t, "secret", got.Local.Password)
	assert.Equal(t, "url_db", got.Local.Database)
	assert.Equal(t, "url_db", got.Runtime.DefaultDatabase)
	assert.Equal(t, "disable", got.Local.SSLMode)
}

func TestResolverIgnoresConfigLoadErrorWhenEnvSuppliesHosts(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "missing.toml")
	got, err := (Resolver{
		StorePath: missing,
		Env: map[string]string{
			"PGSYNC_REMOTE_HOST": "env-remote",
			"PGSYNC_LOCAL_HOST":  "env-local",
		},
		Infisical: &stubInfisical{fn: func(context.Context) (string, error) { return "test-db", nil }},
	}).Resolve(context.Background(), FlagOverrides{})
	require.NoError(t, err)
	assert.Equal(t, "env-remote", got.Remote.Host)
	assert.Equal(t, "env-local", got.Local.Host)
}

func TestResolverIgnoresMalformedConfigWhenFlagsSupplyHosts(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "bad.toml")
	require.NoError(t, os.WriteFile(path, []byte("not = [valid toml"), 0o600))
	got, err := (Resolver{
		StorePath: path,
		Env:       map[string]string{},
		Infisical: &stubInfisical{fn: func(context.Context) (string, error) { return "test-db", nil }},
	}).Resolve(context.Background(), FlagOverrides{
		Remote: config.Connection{Host: "flag-remote"},
		Local:  config.Connection{Host: "flag-local"},
	})
	require.NoError(t, err)
	assert.Equal(t, "flag-remote", got.Remote.Host)
}

func TestResolverReturnsEnvParseError(t *testing.T) {
	t.Parallel()
	_, err := (Resolver{StorePath: writeTestConfig(t, testConfig()), Env: map[string]string{
		"PGSYNC_THREADS": "bad",
	}}).Resolve(context.Background(), FlagOverrides{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apply env")
}

func TestResolverReturnsValidationError(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.Remote.Host = "bad host"
	_, err := (Resolver{StorePath: writeTestConfig(t, cfg), Env: map[string]string{}}).Resolve(context.Background(), FlagOverrides{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "whitespace")
}

func TestResolverDefaultPathErrorIsLoadError(t *testing.T) {
	t.Parallel()
	_, err := (Resolver{Env: map[string]string{}}).Resolve(context.Background(), FlagOverrides{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load config")
}

func TestPlanOptionsFromConfig(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	opts, err := PlanOptionsFromConfig(cfg, " ", SyncFlags{
		Tables:  []string{"users", "orders", "users"},
		DryRun:  true,
		Analyze: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "fixture-db", opts.Database)
	assert.Equal(t, []string{"users", "orders"}, opts.Tables)
	assert.Equal(t, 4, opts.Threads)
	assert.Equal(t, engine.ModeNative, opts.Mode)
	assert.True(t, opts.DryRun)
	assert.True(t, opts.ConcurrentIndexes)
	assert.True(t, opts.Analyze)
}

func TestPlanOptionsFromConfigFallsBackToRemoteDatabase(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.Runtime.DefaultDatabase = ""
	cfg.Remote.Database = "remote-db"
	opts, err := PlanOptionsFromConfig(cfg, " ", SyncFlags{})
	require.NoError(t, err)
	assert.Equal(t, "remote-db", opts.Database)
}

func TestPlanOptionsFromConfigValidationError(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.Remote.Host = ""
	_, err := PlanOptionsFromConfig(cfg, "fixture-db", SyncFlags{})
	assert.Error(t, err)
}

func TestDotEnvParsing(t *testing.T) {
	t.Parallel()
	key, value, ok := parseDotEnvLine("export POSTGRES_URL='postgres://u:p@h/db'")
	assert.True(t, ok)
	assert.Equal(t, "POSTGRES_URL", key)
	assert.Equal(t, "postgres://u:p@h/db", value)
	key, value, ok = parseDotEnvLine(`NUXT_DB_NAME="app"`)
	assert.True(t, ok)
	assert.Equal(t, "NUXT_DB_NAME", key)
	assert.Equal(t, "app", value)
	key, value, ok = parseDotEnvLine("A=value # comment")
	assert.True(t, ok)
	assert.Equal(t, "A", key)
	assert.Equal(t, "value", value)
	_, _, ok = parseDotEnvLine("# comment")
	assert.False(t, ok)
	_, _, ok = parseDotEnvLine("invalid")
	assert.False(t, ok)
	_, _, ok = parseDotEnvLine("=value")
	assert.False(t, ok)
	assert.Empty(t, loadDotEnv(filepath.Join(t.TempDir(), "missing.env")))
}

func TestEnvMap(t *testing.T) {
	t.Parallel()
	assert.Equal(t, map[string]string{"A": "1"}, envMap([]string{"A=1", "ignored"}))
}

func TestSanitizeErrorNil(t *testing.T) {
	t.Parallel()
	assert.NoError(t, sanitizeError(nil, testConfig()))
}

func writeTestConfig(t *testing.T, cfg config.Config) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, config.Save(path, cfg))
	return path
}

func testConfig() config.Config {
	cfg := config.Defaults()
	cfg.Remote = config.Connection{
		Host:     "file-remote",
		Port:     5543,
		User:     "file-user",
		Password: "remote-pass",
		Database: "fixture-db",
		SSLMode:  "require",
		ProxyURL: "socks5://proxy.example.com:1080",
	}
	cfg.Local = config.Connection{
		Host:     "file-local",
		Port:     15432,
		User:     "file-local-user",
		Password: "local-pass",
		Database: "fixture-db",
		SSLMode:  "disable",
	}
	cfg.Runtime.Threads = 4
	cfg.Runtime.Engine = "native"
	cfg.Runtime.UseSystemPgtools = true
	cfg.Runtime.DefaultDatabase = "fixture-db"
	cfg.Runtime.ConcurrentIndexes = true
	cfg.Logging.Level = "debug"
	cfg.Logging.Format = "json"
	return cfg
}

func clearPGSyncEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"PGSYNC_REMOTE_HOST", "PGSYNC_REMOTE_PORT", "PGSYNC_REMOTE_USER", "PGSYNC_REMOTE_PASSWORD",
		"PGSYNC_REMOTE_DATABASE", "PGSYNC_REMOTE_SSL_MODE", "PGSYNC_REMOTE_PROXY_URL",
		"PGSYNC_LOCAL_HOST", "PGSYNC_LOCAL_PORT", "PGSYNC_LOCAL_USER", "PGSYNC_LOCAL_PASSWORD",
		"PGSYNC_LOCAL_SSL_MODE", "PGSYNC_THREADS", "PGSYNC_ENGINE", "PGSYNC_USE_SYSTEM_PGTOOLS",
		"PGSYNC_DEFAULT_DATABASE", "PGSYNC_CONCURRENT_INDEXES", "PGSYNC_LOG_LEVEL", "PGSYNC_LOG_FORMAT", "POSTGRES_URL",
	}
	for _, key := range keys {
		t.Setenv(key, "")
	}
}

func TestExitCode(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, ExitCode(nil))
	assert.Equal(t, 2, ExitCode(ErrNotImplemented))
	assert.Equal(t, 1, ExitCode(errors.New("ordinary")))
}

func TestResolverSkipsInfisicalWhenDatabaseAlreadySet(t *testing.T) {
	t.Parallel()
	called := 0
	r := Resolver{
		StorePath: writeTestConfig(t, testConfig()),
		Env:       map[string]string{},
		Infisical: &stubInfisical{
			fn: func(context.Context) (string, error) {
				called++
				return "should_not_be_called", nil
			},
		},
	}
	cfg, err := r.Resolve(context.Background(), FlagOverrides{
		Remote: config.Connection{Database: "from_flag"},
		Local:  config.Connection{Database: "from_flag"},
	})
	require.NoError(t, err)
	assert.Equal(t, "from_flag", cfg.Remote.Database)
	assert.Equal(t, 0, called)
}

func TestResolverInvokesInfisicalWhenDatabaseUnset(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.Remote.Database = ""
	cfg.Local.Database = ""
	cfg.Runtime.DefaultDatabase = ""

	r := Resolver{
		StorePath: writeTestConfig(t, cfg),
		Env:       map[string]string{},
		Infisical: &stubInfisical{
			fn: func(context.Context) (string, error) { return "resolved_db", nil },
		},
	}
	got, err := r.Resolve(context.Background(), FlagOverrides{})
	require.NoError(t, err)
	assert.Equal(t, "resolved_db", got.Remote.Database)
	assert.Equal(t, "resolved_db", got.Local.Database)
	assert.Equal(t, "resolved_db", got.Runtime.DefaultDatabase)
}

func TestResolverPropagatesInfisicalError(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.Remote.Database = ""
	cfg.Local.Database = ""
	cfg.Runtime.DefaultDatabase = ""
	r := Resolver{
		StorePath: writeTestConfig(t, cfg),
		Env:       map[string]string{},
		Infisical: &stubInfisical{
			fn: func(context.Context) (string, error) {
				return "", errors.New("pgsync: no .infisical.json found")
			},
		},
	}
	_, err := r.Resolve(context.Background(), FlagOverrides{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no .infisical.json")
}

type stubInfisical struct {
	fn func(context.Context) (string, error)
}

func (s *stubInfisical) ResolveDBName(ctx context.Context) (string, error) {
	return s.fn(ctx)
}
