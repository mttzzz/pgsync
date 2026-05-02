package config_test

import (
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
