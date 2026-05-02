package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/config"
)

func TestStoreSaveLoadRoundtrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	want := config.Defaults()
	want.Remote.Host = "prod.example.com"
	want.Remote.User = "readonly"

	require.NoError(t, config.Save(path, want))

	got, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestStoreSaveAtomicAndPerms(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	require.NoError(t, config.Save(path, config.Defaults()))

	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
			"config file must be 0600 on unix")
	}

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "config.toml", entries[0].Name())
}

func TestStoreLoadMissingReturnsErrNotExist(t *testing.T) {
	t.Parallel()
	_, err := config.Load(filepath.Join(t.TempDir(), "nope.toml"))
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestStoreLoadMalformedTOMLReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("not = [valid toml"), 0o600))
	_, err := config.Load(path)
	require.Error(t, err)
}

func TestStoreSaveFailsWhenPathParentIsFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	parentFile := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(parentFile, []byte("x"), 0o600))
	err := config.Save(filepath.Join(parentFile, "config.toml"), config.Defaults())
	require.Error(t, err)
}
