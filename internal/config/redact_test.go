package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mttzzz/pgsync/internal/config"
)

func TestRedactedMasksSecretsWithoutMutation(t *testing.T) {
	t.Parallel()
	cfg := config.Defaults()
	cfg.Remote.Password = "remote-secret"
	cfg.Local.Password = "local-secret"
	cfg.Remote.ProxyURL = "socks5://user:proxy-secret@host:1080"

	got := config.Redacted(cfg)

	assert.Equal(t, "xxxxx", got.Remote.Password)
	assert.Equal(t, "xxxxx", got.Local.Password)
	assert.Equal(t, "socks5://user:xxxxx@host:1080", got.Remote.ProxyURL)
	assert.Equal(t, "remote-secret", cfg.Remote.Password)
	assert.Equal(t, "local-secret", cfg.Local.Password)
	assert.Equal(t, "socks5://user:proxy-secret@host:1080", cfg.Remote.ProxyURL)
}

func TestRedactedKeepsEmptyAndPasswordlessValues(t *testing.T) {
	t.Parallel()
	cfg := config.Defaults()
	cfg.Remote.ProxyURL = "socks5://user@host:1080"

	got := config.Redacted(cfg)

	assert.Empty(t, got.Remote.Password)
	assert.Empty(t, got.Local.Password)
	assert.Equal(t, "socks5://user@host:1080", got.Remote.ProxyURL)
	assert.Empty(t, config.RedactProxyURL(""))
	assert.Equal(t, "://broken", config.RedactProxyURL("://broken"))
}
