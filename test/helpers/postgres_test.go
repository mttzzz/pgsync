package helpers

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresContainerConfig(t *testing.T) {
	t.Parallel()

	pg := PostgresContainer{
		Host:     "127.0.0.1",
		Port:     15432,
		User:     "postgres",
		Password: "secret",
		Database: "tiny",
	}

	cfg := pg.Config()

	assert.Equal(t, "127.0.0.1", cfg.Host)
	assert.Equal(t, 15432, cfg.Port)
	assert.Equal(t, "postgres", cfg.User)
	assert.Equal(t, "secret", cfg.Password)
	assert.Equal(t, "tiny", cfg.Database)
	assert.Equal(t, "disable", cfg.SSLMode)
}

func TestPostgresConnectionString(t *testing.T) {
	t.Parallel()

	pg := PostgresContainer{
		Host:     "localhost",
		Port:     15432,
		User:     "postgres",
		Password: "p@ss:word",
		Database: "tiny",
	}

	connString, err := postgresConnectionString(pg)
	require.NoError(t, err)
	parsed, err := url.Parse(connString)
	require.NoError(t, err)
	password, hasPassword := parsed.User.Password()

	assert.Equal(t, "postgres", parsed.Scheme)
	assert.Equal(t, "localhost:15432", parsed.Host)
	assert.Equal(t, "/tiny", parsed.Path)
	assert.Equal(t, "postgres", parsed.User.Username())
	assert.True(t, hasPassword)
	assert.Equal(t, "p@ss:word", password)
	assert.Equal(t, "disable", parsed.Query().Get("sslmode"))
}
