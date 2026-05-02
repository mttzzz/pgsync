// Package config defines pgsync TOML configuration types and defaults.
package config

import "runtime"

/* Config is the root TOML configuration object. */
type Config struct {
	Remote  Connection `toml:"remote"`
	Local   Connection `toml:"local"`
	Runtime Runtime    `toml:"runtime"`
	Logging Logging    `toml:"logging"`
}

/* Connection describes one PostgreSQL endpoint. */
type Connection struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	User     string `toml:"user"`
	Password string `toml:"password"`
	Database string `toml:"database,omitempty"`
	SSLMode  string `toml:"ssl_mode"`
	ProxyURL string `toml:"proxy_url,omitempty"`
}

/* Runtime contains engine and execution options. */
type Runtime struct {
	Threads           int    `toml:"threads"`
	Engine            string `toml:"engine"`
	UseSystemPgtools  bool   `toml:"use_system_pgtools"`
	DefaultDatabase   string `toml:"default_database,omitempty"`
	ConcurrentIndexes bool   `toml:"concurrent_indexes"`
}

/* Logging controls application log output. */
type Logging struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
}

/* Defaults returns a valid base configuration with conservative defaults. */
func Defaults() Config {
	return Config{
		Remote: Connection{Port: 5432, SSLMode: "require"},
		Local:  Connection{Port: 5432, SSLMode: "disable"},
		Runtime: Runtime{
			Threads: runtime.NumCPU(),
			Engine:  "native",
		},
		Logging: Logging{Level: "info", Format: "text"},
	}
}
