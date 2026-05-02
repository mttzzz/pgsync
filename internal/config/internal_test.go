package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultPathByOS(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		goos string
		env  map[string]string
		want string
	}{
		{
			name: "windows appdata",
			goos: "windows",
			env:  map[string]string{"APPDATA": `C:\Users\u\AppData\Roaming`},
			want: filepath.Join(`C:\Users\u\AppData\Roaming`, appDir, fileName),
		},
		{
			name: "linux xdg",
			goos: "linux",
			env:  map[string]string{"XDG_CONFIG_HOME": "/custom"},
			want: filepath.Join("/custom", appDir, fileName),
		},
		{
			name: "linux home",
			goos: "linux",
			env:  map[string]string{"HOME": "/home/u"},
			want: filepath.Join("/home/u", ".config", appDir, fileName),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := defaultPath(tc.goos, tc.env)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDefaultPathByOSErrors(t *testing.T) {
	t.Parallel()
	_, err := defaultPath("windows", map[string]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "APPDATA")

	_, err = defaultPath("linux", map[string]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HOME")
}

func TestStoreLoadReadError(t *testing.T) {
	t.Parallel()
	_, err := Load(t.TempDir())
	require.Error(t, err)
	assert.NotErrorIs(t, err, os.ErrNotExist)
	assert.Contains(t, err.Error(), "read:")
}

func TestSaveWithOpsErrors(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	cases := []struct {
		name   string
		ops    *fakeSaveOps
		want   string
		remove bool
	}{
		{name: "mkdir", ops: &fakeSaveOps{mkdirErr: boom}, want: "mkdir config dir", remove: false},
		{name: "open", ops: &fakeSaveOps{openErr: boom}, want: "open tmp", remove: false},
		{name: "encode", ops: &fakeSaveOps{file: &fakeSaveFile{writeErr: boom}}, want: "encode", remove: true},
		{name: "sync", ops: &fakeSaveOps{file: &fakeSaveFile{syncErr: boom}}, want: "fsync", remove: true},
		{name: "close", ops: &fakeSaveOps{file: &fakeSaveFile{closeErr: boom}}, want: "close tmp", remove: true},
		{name: "rename", ops: &fakeSaveOps{file: &fakeSaveFile{}, renameErr: boom}, want: "rename", remove: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := saveWithOps(filepath.Join("dir", "config.toml"), Defaults(), tc.ops)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			if tc.remove {
				assert.Equal(t, 1, tc.ops.removeCalls)
			} else {
				assert.Zero(t, tc.ops.removeCalls)
			}
		})
	}
}

func TestRealSaveOpsRemove(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "x")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
	require.NoError(t, realSaveOps{}.Remove(path))
}

type fakeSaveOps struct {
	mkdirErr    error
	openErr     error
	renameErr   error
	file        *fakeSaveFile
	removeCalls int
}

func (f *fakeSaveOps) MkdirAll(path string, perm fs.FileMode) error {
	return f.mkdirErr
}

func (f *fakeSaveOps) OpenFile(path string, flag int, perm fs.FileMode) (syncWriteCloser, error) {
	if f.openErr != nil {
		return nil, f.openErr
	}
	if f.file == nil {
		f.file = &fakeSaveFile{}
	}
	return f.file, nil
}

func (f *fakeSaveOps) Remove(path string) error {
	f.removeCalls++
	return nil
}

func (f *fakeSaveOps) Rename(oldPath, newPath string) error {
	return f.renameErr
}

type fakeSaveFile struct {
	writeErr error
	syncErr  error
	closeErr error
	buf      strings.Builder
}

func (f *fakeSaveFile) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.buf.Write(p)
}

func (f *fakeSaveFile) Sync() error { return f.syncErr }

func (f *fakeSaveFile) Close() error { return f.closeErr }
