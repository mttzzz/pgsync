package config_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mttzzz/pgsync/internal/config"
)

func TestApplyEnv(t *testing.T) {
	t.Parallel()

	base := config.Defaults()
	env := map[string]string{
		"PGSYNC_REMOTE_HOST":        "prod",
		"PGSYNC_REMOTE_PORT":        "6432",
		"PGSYNC_REMOTE_USER":        "ro",
		"PGSYNC_REMOTE_PASSWORD":    "x",
		"PGSYNC_REMOTE_DATABASE":    "ai",
		"PGSYNC_REMOTE_SSL_MODE":    "require",
		"PGSYNC_REMOTE_PROXY_URL":   "socks5://proxy:1080",
		"PGSYNC_LOCAL_HOST":         "localhost",
		"PGSYNC_LOCAL_PORT":         "15432",
		"PGSYNC_LOCAL_USER":         "postgres",
		"PGSYNC_LOCAL_PASSWORD":     "postgres",
		"PGSYNC_LOCAL_SSL_MODE":     "disable",
		"PGSYNC_THREADS":            "16",
		"PGSYNC_ENGINE":             "external",
		"PGSYNC_USE_SYSTEM_PGTOOLS": "true",
		"PGSYNC_DEFAULT_DATABASE":   "ai",
		"PGSYNC_CONCURRENT_INDEXES": "true",
		"PGSYNC_LOG_LEVEL":          "debug",
		"PGSYNC_LOG_FORMAT":         "json",
		"OTHER":                     "ignored",
	}
	got, err := config.ApplyEnv(base, env)
	assert.NoError(t, err)
	assert.Equal(t, "prod", got.Remote.Host)
	assert.Equal(t, 6432, got.Remote.Port)
	assert.Equal(t, "ro", got.Remote.User)
	assert.Equal(t, "x", got.Remote.Password)
	assert.Equal(t, "ai", got.Remote.Database)
	assert.Equal(t, "socks5://proxy:1080", got.Remote.ProxyURL)
	assert.Equal(t, "localhost", got.Local.Host)
	assert.Equal(t, 15432, got.Local.Port)
	assert.Equal(t, "postgres", got.Local.User)
	assert.Equal(t, "postgres", got.Local.Password)
	assert.Equal(t, "disable", got.Local.SSLMode)
	assert.Equal(t, 16, got.Runtime.Threads)
	assert.Equal(t, "external", got.Runtime.Engine)
	assert.True(t, got.Runtime.UseSystemPgtools)
	assert.True(t, got.Runtime.ConcurrentIndexes)
	assert.Equal(t, "debug", got.Logging.Level)
	assert.Equal(t, "json", got.Logging.Format)
}

func TestApplyEnvPostgresURL(t *testing.T) {
	t.Parallel()
	got, err := config.ApplyEnv(config.Defaults(), map[string]string{
		"POSTGRES_URL": strings.Join([]string{"postgresql://app:", "sec", "ret", "@db.example.com:6543/my%20db?sslmode=verify-full"}, ""),
	})
	assert.NoError(t, err)
	assert.Equal(t, "db.example.com", got.Local.Host)
	assert.Equal(t, 6543, got.Local.Port)
	assert.Equal(t, "app", got.Local.User)
	assert.Equal(t, "secret", got.Local.Password)
	assert.Equal(t, "my db", got.Local.Database)
	assert.Equal(t, "my db", got.Runtime.DefaultDatabase)
	assert.Equal(t, "verify-full", got.Local.SSLMode)
}

func TestApplyEnvPostgresURLCanBeOverriddenByPGSyncEnv(t *testing.T) {
	t.Parallel()
	got, err := config.ApplyEnv(config.Defaults(), map[string]string{
		"POSTGRES_URL":              strings.Join([]string{"postgres://app:", "sec", "ret", "@db.example.com/from_url"}, ""),
		"PGSYNC_LOCAL_HOST":         "override.example.com",
		"PGSYNC_DEFAULT_DATABASE":   "override_db",
		"PGSYNC_LOCAL_PASSWORD":     "override-secret",
		"PGSYNC_LOCAL_SSL_MODE":     "require",
		"PGSYNC_REMOTE_PROXY_URL":   "socks5://proxy:1080",
		"PGSYNC_CONCURRENT_INDEXES": "true",
	})
	assert.NoError(t, err)
	assert.Equal(t, "override.example.com", got.Local.Host)
	assert.Equal(t, "override-secret", got.Local.Password)
	assert.Equal(t, "from_url", got.Local.Database)
	assert.Equal(t, "override_db", got.Runtime.DefaultDatabase)
	assert.Equal(t, "require", got.Local.SSLMode)
}

func TestApplyEnvPostgresURLDefaultsOptionalParts(t *testing.T) {
	t.Parallel()
	got, err := config.ApplyEnv(config.Defaults(), map[string]string{
		"POSTGRES_URL": "postgres://db.example.com",
	})
	assert.NoError(t, err)
	assert.Equal(t, "db.example.com", got.Local.Host)
	assert.Equal(t, 5432, got.Local.Port)
	assert.Empty(t, got.Local.User)
	assert.Empty(t, got.Local.Password)
	assert.Empty(t, got.Local.Database)
	assert.Empty(t, got.Runtime.DefaultDatabase)
}

func TestApplyEnvBadPostgresURL(t *testing.T) {
	t.Parallel()
	for _, rawURL := range []string{
		strings.Join([]string{"mysql://app:", "sec", "ret", "@db.example.com/app"}, ""),
		"postgres:///app",
		"postgres://db.example.com:bad/app",
		"%",
	} {
		rawURL := rawURL
		t.Run(rawURL, func(t *testing.T) {
			t.Parallel()
			_, err := config.ApplyEnv(config.Defaults(), map[string]string{"POSTGRES_URL": rawURL})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "POSTGRES_URL")
			assert.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestApplyEnvBadInt(t *testing.T) {
	t.Parallel()
	_, err := config.ApplyEnv(config.Defaults(), map[string]string{
		"PGSYNC_REMOTE_PORT": "not-a-number",
	})
	assert.Error(t, err)
}

func TestApplyEnvBadBool(t *testing.T) {
	t.Parallel()
	_, err := config.ApplyEnv(config.Defaults(), map[string]string{
		"PGSYNC_USE_SYSTEM_PGTOOLS": "not-a-bool",
	})
	assert.Error(t, err)
}

func TestApplyEnvNoOpsForBlank(t *testing.T) {
	t.Parallel()
	base := config.Defaults()
	base.Remote.Host = "kept"
	got, err := config.ApplyEnv(base, map[string]string{
		"PGSYNC_REMOTE_HOST": "",
	})
	assert.NoError(t, err)
	assert.Equal(t, "kept", got.Remote.Host)
}
