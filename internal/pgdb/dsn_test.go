package pgdb_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/config"
	"github.com/mttzzz/pgsync/internal/pgdb"
)

func TestEndpointFromConfigAppliesOverrideAndIgnoresProxy(t *testing.T) {
	t.Parallel()
	cfg := config.Connection{
		Host:     "prod.example.com",
		Port:     6432,
		User:     "alice",
		Password: "secret",
		Database: "configured",
		SSLMode:  "require",
		ProxyURL: "socks5://proxy.example.com:1080",
	}

	ep := pgdb.EndpointFromConfig(cfg, "requested")
	connString, err := pgdb.BuildConnString(ep)
	require.NoError(t, err)

	assert.Equal(t, "requested", ep.Database)
	assert.NotContains(t, connString, "proxy.example.com")
}

func TestEndpointFromConfigUsesConfiguredDatabaseWithoutOverride(t *testing.T) {
	t.Parallel()
	cfg := config.Connection{Database: "configured"}

	ep := pgdb.EndpointFromConfig(cfg, "")

	assert.Equal(t, "configured", ep.Database)
}

func TestBuildConnStringRemoteIncludesFieldsAndPassword(t *testing.T) {
	t.Parallel()
	ep := pgdb.Endpoint{
		Host:     "prod.example.com",
		Port:     6543,
		User:     "user@example.com",
		Password: "p@ss:word",
		Database: "appdb",
		SSLMode:  "verify-full",
	}

	connString, err := pgdb.BuildConnString(ep)
	require.NoError(t, err)
	parsed, err := url.Parse(connString)
	require.NoError(t, err)
	password, hasPassword := parsed.User.Password()

	assert.Equal(t, "postgres", parsed.Scheme)
	assert.Equal(t, "prod.example.com:6543", parsed.Host)
	assert.Equal(t, "/appdb", parsed.Path)
	assert.Equal(t, "user@example.com", parsed.User.Username())
	assert.True(t, hasPassword)
	assert.Equal(t, "p@ss:word", password)
	assert.Equal(t, "verify-full", parsed.Query().Get("sslmode"))
}

func TestBuildConnStringWithoutPasswordUsesUserOnly(t *testing.T) {
	t.Parallel()
	ep := pgdb.Endpoint{Host: "localhost", Port: 5432, User: "postgres", Database: "appdb"}

	connString, err := pgdb.BuildConnString(ep)
	require.NoError(t, err)
	parsed, err := url.Parse(connString)
	require.NoError(t, err)
	_, hasPassword := parsed.User.Password()

	assert.Equal(t, "postgres", parsed.User.Username())
	assert.False(t, hasPassword)
}

func TestBuildConnStringLocalMaintenanceDefaultsToPostgres(t *testing.T) {
	t.Parallel()
	ep := pgdb.Endpoint{Host: "127.0.0.1", Port: 5432, User: "postgres", SSLMode: "disable"}

	connString, err := pgdb.BuildConnString(ep)
	require.NoError(t, err)
	parsed, err := url.Parse(connString)
	require.NoError(t, err)

	assert.Equal(t, "/postgres", parsed.Path)
	assert.Equal(t, "disable", parsed.Query().Get("sslmode"))
}

func TestBuildConnStringRejectsInvalidHostAndPort(t *testing.T) {
	t.Parallel()
	cases := map[string]pgdb.Endpoint{
		"missing host": {Port: 5432},
		"blank host":   {Host: " \t", Port: 5432},
		"zero port":    {Host: "localhost"},
		"high port":    {Host: "localhost", Port: 65536},
	}

	for name, ep := range cases {
		_, err := pgdb.BuildConnString(ep)
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "postgres", name)
	}
}

func TestMaskConnStringRedactsURLPassword(t *testing.T) {
	t.Parallel()
	connString, err := pgdb.BuildConnString(pgdb.Endpoint{
		Host:     "prod.example.com",
		Port:     5432,
		User:     "alice",
		Password: "redactable-value",
		Database: "app",
		SSLMode:  "require",
	})
	require.NoError(t, err)

	masked := pgdb.MaskConnString(connString)

	assert.NotContains(t, masked, "redactable-value")
	assert.Contains(t, masked, "alice")
	assert.Contains(t, masked, "xxxxx")
}

func TestMaskConnStringRedactsKeywordPassword(t *testing.T) {
	t.Parallel()
	connString := "host=prod.example.com user=alice password=super-secret dbname=app"

	masked := pgdb.MaskConnString(connString)

	assert.NotContains(t, masked, "super-secret")
	assert.Equal(t, "host=prod.example.com user=alice password=xxxxx dbname=app", masked)
}

func TestMaskConnStringHandlesEmptyAndPasswordlessValues(t *testing.T) {
	t.Parallel()

	assert.Empty(t, pgdb.MaskConnString(""))
	assert.Equal(t, "postgres://alice@localhost:5432/app", pgdb.MaskConnString("postgres://alice@localhost:5432/app"))
}
