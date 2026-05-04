// Package cli implements the pgsync command-line interface.
package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/engine"
)

// Resolver resolves configuration from defaults, a TOML file, environment, and CLI flags.
type Resolver struct {
	StorePath string
	Env       map[string]string
}

// FlagOverrides contains command-line values that may override config/env values.
type FlagOverrides struct {
	ConfigPath       string
	Threads          int
	Engine           string
	UseSystemPgtools *bool
	Output           string
	Quiet            bool
	Verbose          bool
	NoColor          bool
	Remote           config.Connection
	Local            config.Connection
}

// SyncFlags contains sync-command-specific options.
type SyncFlags struct {
	Tables  []string
	DryRun  bool
	Analyze bool
}

// Resolve resolves configuration using .env plus the process environment.
func Resolve(ctx context.Context, flags FlagOverrides) (config.Config, error) {
	return Resolver{Env: processEnv()}.Resolve(ctx, flags)
}

// Resolve resolves configuration from defaults, optional store, env, and flags.
func (r Resolver) Resolve(ctx context.Context, flags FlagOverrides) (config.Config, error) {
	if err := ctx.Err(); err != nil {
		return config.Config{}, err
	}

	env := r.env()
	cfg := config.Defaults()
	loaded, loadErr := r.load(env, flags.ConfigPath)
	if loadErr == nil {
		cfg = overlayConfig(cfg, loaded)
	}

	var err error
	cfg, err = config.ApplyEnv(cfg, env)
	if err != nil {
		return config.Config{}, fmt.Errorf("apply env: %w", err)
	}
	cfg = applyFlagOverrides(cfg, flags)

	if loadErr != nil && !connectionHostsProvided(env, flags) {
		return config.Config{}, fmt.Errorf("load config: %w", loadErr)
	}
	if err := config.Validate(cfg); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

// PlanOptionsFromConfig converts resolved config and sync flags into engine options.
func PlanOptionsFromConfig(cfg config.Config, db string, syncFlags SyncFlags) (engine.PlanOptions, error) {
	database := strings.TrimSpace(db)
	if database == "" {
		database = strings.TrimSpace(cfg.Runtime.DefaultDatabase)
	}
	if database == "" {
		database = strings.TrimSpace(cfg.Remote.Database)
	}
	opts := engine.PlanOptions{
		Remote:            cfg.Remote,
		Local:             cfg.Local,
		Database:          database,
		Tables:            syncFlags.Tables,
		Threads:           cfg.Runtime.Threads,
		Mode:              engine.Mode(cfg.Runtime.Engine),
		UseSystemPgtools:  cfg.Runtime.UseSystemPgtools,
		DryRun:            syncFlags.DryRun,
		ConcurrentIndexes: cfg.Runtime.ConcurrentIndexes,
		Analyze:           syncFlags.Analyze,
	}
	if err := opts.Validate(); err != nil {
		return engine.PlanOptions{}, err
	}
	return opts, nil
}

func (r Resolver) env() map[string]string {
	if r.Env != nil {
		return r.Env
	}
	return envMap(os.Environ())
}

func (r Resolver) load(env map[string]string, configPath string) (config.Config, error) {
	path := configPath
	if path == "" {
		path = r.StorePath
	}
	if path == "" {
		var err error
		path, err = config.DefaultPath(env)
		if err != nil {
			return config.Config{}, err
		}
	}
	return config.Load(path)
}

func applyFlagOverrides(cfg config.Config, flags FlagOverrides) config.Config {
	cfg.Remote = overlayConnection(cfg.Remote, flags.Remote)
	cfg.Local = overlayConnection(cfg.Local, flags.Local)
	if flags.Threads > 0 {
		cfg.Runtime.Threads = flags.Threads
	}
	if flags.Engine != "" {
		cfg.Runtime.Engine = flags.Engine
	}
	if flags.UseSystemPgtools != nil {
		cfg.Runtime.UseSystemPgtools = *flags.UseSystemPgtools
	}
	return cfg
}

func overlayConfig(base config.Config, override config.Config) config.Config {
	base.Remote = overlayConnection(base.Remote, override.Remote)
	base.Local = overlayConnection(base.Local, override.Local)
	base.Runtime = overlayRuntime(base.Runtime, override.Runtime)
	base.Logging = overlayLogging(base.Logging, override.Logging)
	return base
}

func overlayConnection(base config.Connection, override config.Connection) config.Connection {
	if override.Host != "" {
		base.Host = override.Host
	}
	if override.Port != 0 {
		base.Port = override.Port
	}
	if override.User != "" {
		base.User = override.User
	}
	if override.Password != "" {
		base.Password = override.Password
	}
	if override.Database != "" {
		base.Database = override.Database
	}
	if override.SSLMode != "" {
		base.SSLMode = override.SSLMode
	}
	if override.ProxyURL != "" {
		base.ProxyURL = override.ProxyURL
	}
	return base
}

func overlayRuntime(base config.Runtime, override config.Runtime) config.Runtime {
	if override.Threads != 0 {
		base.Threads = override.Threads
	}
	if override.Engine != "" {
		base.Engine = override.Engine
	}
	if override.UseSystemPgtools {
		base.UseSystemPgtools = true
	}
	if override.DefaultDatabase != "" {
		base.DefaultDatabase = override.DefaultDatabase
	}
	if override.ConcurrentIndexes {
		base.ConcurrentIndexes = true
	}
	return base
}

func overlayLogging(base config.Logging, override config.Logging) config.Logging {
	if override.Level != "" {
		base.Level = override.Level
	}
	if override.Format != "" {
		base.Format = override.Format
	}
	return base
}

func connectionHostsProvided(env map[string]string, flags FlagOverrides) bool {
	remote := strings.TrimSpace(flags.Remote.Host)
	if remote == "" {
		remote = strings.TrimSpace(env["PGSYNC_REMOTE_HOST"])
	}
	local := strings.TrimSpace(flags.Local.Host)
	if local == "" {
		local = strings.TrimSpace(env["PGSYNC_LOCAL_HOST"])
	}
	if local == "" {
		local = strings.TrimSpace(env["POSTGRES_URL"])
	}
	return remote != "" && local != ""
}

func processEnv() map[string]string {
	env := loadDotEnv(".env")
	for key, value := range envMap(os.Environ()) {
		if strings.TrimSpace(value) == "" {
			continue
		}
		env[key] = value
	}
	return env
}

func loadDotEnv(path string) map[string]string {
	content, err := os.ReadFile(path) // #nosec G304 -- .env is intentionally loaded from the caller's working directory.
	if err != nil {
		return map[string]string{}
	}
	env := make(map[string]string)
	for _, line := range strings.Split(string(content), "\n") {
		key, value, ok := parseDotEnvLine(line)
		if ok {
			env[key] = value
		}
	}
	return env
}

func parseDotEnvLine(line string) (string, string, bool) {
	line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", false
	}
	return key, parseDotEnvValue(value), true
}

func parseDotEnvValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		quote := value[0]
		if (quote == '\'' || quote == '"') && value[len(value)-1] == quote {
			return value[1 : len(value)-1]
		}
	}
	if before, _, ok := strings.Cut(value, " #"); ok {
		value = before
	}
	return strings.TrimSpace(value)
}

func envMap(entries []string) map[string]string {
	env := make(map[string]string, len(entries))
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}
