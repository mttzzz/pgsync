package config

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

/* ApplyEnv overlays PGSYNC_* environment variables onto cfg, returning a new
 * Config. Empty values are ignored, so partial env merges with file config.
 */
func ApplyEnv(cfg Config, env map[string]string) (Config, error) {
	mustInt := func(target *int) func(string) error {
		return func(s string) error {
			value, err := strconv.Atoi(s)
			if err != nil {
				return fmt.Errorf("parse int: %w", err)
			}
			*target = value
			return nil
		}
	}
	mustBool := func(target *bool) func(string) error {
		return func(s string) error {
			value, err := strconv.ParseBool(s)
			if err != nil {
				return fmt.Errorf("parse bool: %w", err)
			}
			*target = value
			return nil
		}
	}
	mustStr := func(target *string) func(string) error {
		return func(s string) error {
			*target = s
			return nil
		}
	}

	var err error
	cfg, err = applyConventionalEnv(cfg, env)
	if err != nil {
		return Config{}, err
	}

	var bindErr error
	cfg, bindErr = applyPGSyncBindings(cfg, env, mustInt, mustBool, mustStr)
	if bindErr != nil {
		return Config{}, bindErr
	}
	return cfg, nil
}

/* applyConventionalEnv applies generic conventions: DB_DATABASE (Laravel/Symfony)
 * fills only the database name; POSTGRES_URL fills the entire local connection.
 * Applied in this order so a full URL overrides DB_DATABASE when both are set. */
func applyConventionalEnv(cfg Config, env map[string]string) (Config, error) {
	if dbName := strings.TrimSpace(env["DB_DATABASE"]); dbName != "" {
		cfg.Local.Database = dbName
		cfg.Runtime.DefaultDatabase = dbName
	}
	if rawURL := strings.TrimSpace(env["POSTGRES_URL"]); rawURL != "" {
		var err error
		cfg, err = applyPostgresURL(cfg, rawURL)
		if err != nil {
			return Config{}, fmt.Errorf("env POSTGRES_URL: %w", err)
		}
	}
	return cfg, nil
}

func applyPGSyncBindings(
	cfg Config,
	env map[string]string,
	mustInt func(*int) func(string) error,
	mustBool func(*bool) func(string) error,
	mustStr func(*string) func(string) error,
) (Config, error) {
	type binding struct {
		key string
		set func(string) error
	}
	bindings := []binding{
		{"PGSYNC_REMOTE_HOST", mustStr(&cfg.Remote.Host)},
		{"PGSYNC_REMOTE_PORT", mustInt(&cfg.Remote.Port)},
		{"PGSYNC_REMOTE_USER", mustStr(&cfg.Remote.User)},
		{"PGSYNC_REMOTE_PASSWORD", mustStr(&cfg.Remote.Password)},
		{"PGSYNC_REMOTE_DATABASE", mustStr(&cfg.Remote.Database)},
		{"PGSYNC_REMOTE_SSL_MODE", mustStr(&cfg.Remote.SSLMode)},
		{"PGSYNC_REMOTE_PROXY_URL", mustStr(&cfg.Remote.ProxyURL)},
		{"PGSYNC_LOCAL_HOST", mustStr(&cfg.Local.Host)},
		{"PGSYNC_LOCAL_PORT", mustInt(&cfg.Local.Port)},
		{"PGSYNC_LOCAL_USER", mustStr(&cfg.Local.User)},
		{"PGSYNC_LOCAL_PASSWORD", mustStr(&cfg.Local.Password)},
		{"PGSYNC_LOCAL_SSL_MODE", mustStr(&cfg.Local.SSLMode)},
		{"PGSYNC_THREADS", mustInt(&cfg.Runtime.Threads)},
		{"PGSYNC_ENGINE", mustStr(&cfg.Runtime.Engine)},
		{"PGSYNC_USE_SYSTEM_PGTOOLS", mustBool(&cfg.Runtime.UseSystemPgtools)},
		{"PGSYNC_DEFAULT_DATABASE", mustStr(&cfg.Runtime.DefaultDatabase)},
		{"PGSYNC_CONCURRENT_INDEXES", mustBool(&cfg.Runtime.ConcurrentIndexes)},
		{"PGSYNC_LOG_LEVEL", mustStr(&cfg.Logging.Level)},
		{"PGSYNC_LOG_FORMAT", mustStr(&cfg.Logging.Format)},
	}

	for _, bind := range bindings {
		value, ok := env[bind.key]
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		if err := bind.set(value); err != nil {
			return Config{}, fmt.Errorf("env %s=%q: %w", bind.key, value, err)
		}
	}
	return cfg, nil
}

//nolint:gocyclo // URL parsing maps optional connection components into config explicitly.
func applyPostgresURL(cfg Config, rawURL string) (Config, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return Config{}, fmt.Errorf("parse postgres url: %w", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return Config{}, fmt.Errorf("postgres url scheme must be postgres or postgresql, got %q", parsed.Scheme)
	}
	if strings.TrimSpace(parsed.Hostname()) == "" {
		return Config{}, fmt.Errorf("postgres url host is required")
	}

	cfg.Local.Host = parsed.Hostname()
	cfg.Local.Port = 5432
	if port := parsed.Port(); port != "" {
		cfg.Local.Port, _ = strconv.Atoi(port)
	}
	if parsed.User != nil {
		cfg.Local.User = parsed.User.Username()
		if password, ok := parsed.User.Password(); ok {
			cfg.Local.Password = password
		}
	}
	if database := postgresURLDatabase(parsed); database != "" {
		cfg.Local.Database = database
		cfg.Runtime.DefaultDatabase = database
	}
	if sslMode := strings.TrimSpace(parsed.Query().Get("sslmode")); sslMode != "" {
		cfg.Local.SSLMode = sslMode
	}
	return cfg, nil
}

func postgresURLDatabase(parsed *url.URL) string {
	return strings.TrimPrefix(parsed.Path, "/")
}
