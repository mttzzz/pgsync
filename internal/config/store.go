package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

/* Save writes cfg to path atomically: write to <path>.tmp, fsync, rename.
 * On Unix, file mode is 0600. The parent dir is created with 0700 if missing.
 */
func Save(path string, cfg Config) error {
	return saveWithOps(path, cfg, realSaveOps{})
}

type syncWriteCloser interface {
	io.Writer
	Sync() error
	Close() error
}

type saveOps interface {
	MkdirAll(path string, perm fs.FileMode) error
	OpenFile(path string, flag int, perm fs.FileMode) (syncWriteCloser, error)
	Remove(path string) error
	Rename(oldPath, newPath string) error
}

type realSaveOps struct{}

func (realSaveOps) MkdirAll(path string, perm fs.FileMode) error { return os.MkdirAll(path, perm) }

func (realSaveOps) OpenFile(path string, flag int, perm fs.FileMode) (syncWriteCloser, error) {
	return os.OpenFile(path, flag, perm) // #nosec G304 -- config paths are explicit user/application inputs.
}

func (realSaveOps) Remove(path string) error { return os.Remove(path) }

func (realSaveOps) Rename(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }

func saveWithOps(path string, cfg Config, ops saveOps) error {
	dir := filepath.Dir(path)
	if err := ops.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}

	tmp := path + ".tmp"
	file, err := ops.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open tmp: %w", err)
	}

	if err := toml.NewEncoder(file).Encode(cfg); err != nil {
		_ = file.Close()
		_ = ops.Remove(tmp)
		return fmt.Errorf("encode: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = ops.Remove(tmp)
		return fmt.Errorf("fsync: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = ops.Remove(tmp)
		return fmt.Errorf("close tmp: %w", err)
	}
	if err := ops.Rename(tmp, path); err != nil {
		_ = ops.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

/* Load reads and parses the config from path. Returns os.ErrNotExist wrapped. */
func Load(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path) // #nosec G304 -- config paths are explicit user/application inputs.
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Config{}, err
		}
		return Config{}, fmt.Errorf("read: %w", err)
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return Config{}, fmt.Errorf("decode: %w", err)
	}
	return cfg, nil
}
