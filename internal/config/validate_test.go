package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mttzzz/pgsync/internal/config"
)

func TestValidateHost(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"":                 false,
		"localhost":        true,
		"prod.example.com": true,
		"10.0.0.1":         true,
		"with space":       false,
		"::1":              true,
	}
	for in, ok := range cases {
		err := config.ValidateHost(in)
		if ok {
			assert.NoError(t, err, "host=%q", in)
		} else {
			assert.Error(t, err, "host=%q", in)
		}
	}
}

func TestValidatePort(t *testing.T) {
	t.Parallel()
	assert.NoError(t, config.ValidatePort(5432))
	assert.NoError(t, config.ValidatePort(1))
	assert.NoError(t, config.ValidatePort(65535))
	assert.Error(t, config.ValidatePort(0))
	assert.Error(t, config.ValidatePort(65536))
	assert.Error(t, config.ValidatePort(-1))
}

func TestValidateSSLMode(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"disable", "require", "verify-ca", "verify-full"} {
		assert.NoError(t, config.ValidateSSLMode(mode))
	}
	assert.Error(t, config.ValidateSSLMode("yes"))
	assert.Error(t, config.ValidateSSLMode(""))
}

func TestValidateProxyURL(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"",
		"socks5://proxy:1080",
		"socks5h://proxy:1080",
		"http://proxy:8080",
		"https://proxy:8080",
	} {
		assert.NoError(t, config.ValidateProxyURL(raw), "url=%q", raw)
	}
	for _, raw := range []string{
		"ftp://x",
		"socks4://x",
		"://broken",
		"http:///missing-host",
	} {
		assert.Error(t, config.ValidateProxyURL(raw), "url=%q", raw)
	}
}

func TestValidateAll(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	assert.NoError(t, config.Validate(cfg))

	bad := validConfig()
	bad.Remote.Port = 0
	assert.Error(t, config.Validate(bad))
}

func TestValidateAllRuntimeAndLoggingErrors(t *testing.T) {
	t.Parallel()
	cases := []func(*config.Config){
		func(cfg *config.Config) { cfg.Runtime.Threads = 0 },
		func(cfg *config.Config) { cfg.Runtime.Engine = "bad" },
		func(cfg *config.Config) { cfg.Logging.Level = "bad" },
		func(cfg *config.Config) { cfg.Logging.Format = "bad" },
	}
	for _, mutate := range cases {
		cfg := validConfig()
		mutate(&cfg)
		assert.Error(t, config.Validate(cfg))
	}
}

func validConfig() config.Config {
	cfg := config.Defaults()
	cfg.Remote.Host = "prod"
	cfg.Remote.User = "u"
	cfg.Local.Host = "localhost"
	cfg.Local.User = "postgres"
	return cfg
}
