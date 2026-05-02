package config_test

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/config"
)

func TestConfigRoundtrip(t *testing.T) {
	t.Parallel()

	src := config.Config{
		Remote: config.Connection{
			Host:     "prod.example.com",
			Port:     5432,
			User:     "readonly",
			Password: "secret",
			Database: "ai_pushka_biz",
			SSLMode:  "require",
			ProxyURL: "socks5://proxy:1080",
		},
		Local: config.Connection{
			Host:     "localhost",
			Port:     5432,
			User:     "postgres",
			Password: "postgres",
			SSLMode:  "disable",
		},
		Runtime: config.Runtime{
			Threads:           8,
			Engine:            "native",
			UseSystemPgtools:  false,
			DefaultDatabase:   "ai_pushka_biz",
			ConcurrentIndexes: false,
		},
		Logging: config.Logging{Level: "info", Format: "text"},
	}

	var buf strings.Builder
	require.NoError(t, toml.NewEncoder(&buf).Encode(src))

	var got config.Config
	_, err := toml.Decode(buf.String(), &got)
	require.NoError(t, err)
	assert.Equal(t, src, got)
}

func TestConfigDefaults(t *testing.T) {
	t.Parallel()
	got := config.Defaults()
	assert.Equal(t, 5432, got.Remote.Port)
	assert.Equal(t, 5432, got.Local.Port)
	assert.Equal(t, "require", got.Remote.SSLMode)
	assert.Equal(t, "disable", got.Local.SSLMode)
	assert.Equal(t, "native", got.Runtime.Engine)
	assert.Equal(t, "info", got.Logging.Level)
	assert.Equal(t, "text", got.Logging.Format)
	assert.Greater(t, got.Runtime.Threads, 0)
}
