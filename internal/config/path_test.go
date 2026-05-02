package config_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/config"
)

func TestDefaultPath(t *testing.T) {
	t.Parallel()

	got, err := config.DefaultPath(map[string]string{
		"HOME":            "/home/u",
		"XDG_CONFIG_HOME": "",
		"APPDATA":         "C:\\Users\\u\\AppData\\Roaming",
	})
	require.NoError(t, err)

	if runtime.GOOS == "windows" {
		assert.Equal(t, filepath.Join("C:\\Users\\u\\AppData\\Roaming", "pgsync", "config.toml"), got)
	} else {
		assert.Equal(t, filepath.Join("/home/u", ".config", "pgsync", "config.toml"), got)
	}
}

func TestDefaultPathHonorsXDG(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("XDG not used on windows")
	}
	got, err := config.DefaultPath(map[string]string{
		"HOME":            "/home/u",
		"XDG_CONFIG_HOME": "/custom/cfg",
	})
	require.NoError(t, err)
	assert.Equal(t, "/custom/cfg/pgsync/config.toml", got)
}

func TestDefaultPathMissingEnv(t *testing.T) {
	t.Parallel()
	_, err := config.DefaultPath(map[string]string{})
	require.Error(t, err)
}
