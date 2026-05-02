package pgtools_test

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mttzzz/pgsync/internal/engine/pgtools"
)

type fakeLooker struct {
	paths map[string]string
}

func (f fakeLooker) LookPath(file string) (string, error) {
	if path, ok := f.paths[file]; ok {
		return path, nil
	}
	return "", errors.New("not found")
}

func TestSystemLocatorFound(t *testing.T) {
	t.Parallel()
	binDump := filepath.Join("/usr/bin", pgtools.BinDump())
	binRestore := filepath.Join("/usr/bin", pgtools.BinRestore())
	loc := pgtools.NewSystemLocator(fakeLooker{
		paths: map[string]string{
			pgtools.BinDump():    binDump,
			pgtools.BinRestore(): binRestore,
		},
	})
	dump, err := loc.PgDump()
	require.NoError(t, err)
	assert.Equal(t, binDump, dump)

	restore, err := loc.PgRestore()
	require.NoError(t, err)
	assert.Equal(t, binRestore, restore)
}

func TestSystemLocatorMissing(t *testing.T) {
	t.Parallel()
	loc := pgtools.NewSystemLocator(fakeLooker{paths: map[string]string{}})
	_, err := loc.PgDump()
	require.Error(t, err)
}

func TestExecLookerMissing(t *testing.T) {
	t.Parallel()
	_, err := (pgtools.ExecLooker{}).LookPath("definitely-not-a-real-pgsync-test-binary")
	require.Error(t, err)
}

func TestBinNamesPlatform(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		assert.Equal(t, "pg_dump.exe", pgtools.BinDump())
		assert.Equal(t, "pg_restore.exe", pgtools.BinRestore())
	} else {
		assert.Equal(t, "pg_dump", pgtools.BinDump())
		assert.Equal(t, "pg_restore", pgtools.BinRestore())
	}
}
