package fsx_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/fsx"
)

func TestOSWriteRead(t *testing.T) {
	t.Parallel()
	f := fsx.NewOS()
	path := filepath.Join(t.TempDir(), "x.bin")
	require.NoError(t, f.WriteFile(path, []byte("hello"), 0o644))
	got, err := f.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), got)
}

func TestOSStatNotExist(t *testing.T) {
	t.Parallel()
	f := fsx.NewOS()
	_, err := f.Stat(filepath.Join(t.TempDir(), "nope"))
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestOSMkdirAllRenameAndRemove(t *testing.T) {
	t.Parallel()
	f := fsx.NewOS()
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b")
	require.NoError(t, f.MkdirAll(nested, 0o755))
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	require.NoError(t, f.WriteFile(src, []byte("x"), 0o644))
	require.NoError(t, f.Rename(src, dst))
	_, err := f.Stat(dst)
	require.NoError(t, err)
	require.NoError(t, f.Remove(dst))
	_, err = f.Stat(dst)
	assert.ErrorIs(t, err, os.ErrNotExist)
}
