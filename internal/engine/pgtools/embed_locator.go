package pgtools

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// PayloadSource provides embedded files for a platform.
type PayloadSource interface {
	Platform() string
	Files() map[string][]byte
}

// EmbeddedLocator extracts embedded pg tools into a cache directory.
type EmbeddedLocator struct {
	Source   PayloadSource
	CacheDir string
}

// NewEmbeddedLocator creates an embedded-tools locator.
func NewEmbeddedLocator(source PayloadSource, cacheDir string) *EmbeddedLocator {
	return &EmbeddedLocator{Source: source, CacheDir: cacheDir}
}

// PgDump returns the extracted pg_dump path.
func (e *EmbeddedLocator) PgDump() (string, error) { return e.lookup(BinDump()) }

// PgRestore returns the extracted pg_restore path.
func (e *EmbeddedLocator) PgRestore() (string, error) { return e.lookup(BinRestore()) }

func (e *EmbeddedLocator) lookup(name string) (string, error) {
	if err := e.Extract(); err != nil {
		return "", err
	}
	path := filepath.Join(e.targetDir(), name)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("embedded %s not found: %w", name, err)
	}
	return path, nil
}

// Extract writes all embedded files into the cache directory.
func (e *EmbeddedLocator) Extract() error {
	if e.Source == nil {
		return fmt.Errorf("embedded pgtools source is nil")
	}
	files := e.Source.Files()
	if len(files) == 0 {
		return fmt.Errorf("embedded pgtools payload is empty")
	}
	target := e.targetDir()
	if err := os.MkdirAll(target, 0o700); err != nil {
		return fmt.Errorf("mkdir pgtools cache: %w", err)
	}
	for name, data := range files {
		if err := writeEmbeddedFile(filepath.Join(target, filepath.Base(name)), data); err != nil {
			return err
		}
	}
	return nil
}

func (e *EmbeddedLocator) targetDir() string {
	base := e.CacheDir
	if base == "" {
		base = filepath.Join(os.TempDir(), "pgsync-pgtools")
	}
	return filepath.Join(base, e.Source.Platform(), payloadHash(e.Source.Files()))
}

func writeEmbeddedFile(path string, data []byte) error {
	perm := os.FileMode(0o700)
	if runtime.GOOS == "windows" {
		perm = 0o600
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		return fmt.Errorf("write embedded file: %w", err)
	}
	return nil
}

func payloadHash(files map[string][]byte) string {
	h := sha256.New()
	for name, data := range files {
		h.Write([]byte(name))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}
