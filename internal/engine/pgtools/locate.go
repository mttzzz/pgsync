// Package pgtools resolves paths to pg_dump and pg_restore.
package pgtools

import (
	"fmt"
	"os/exec"
	"runtime"
)

/* Looker abstracts exec.LookPath for tests. */
type Looker interface {
	LookPath(file string) (string, error)
}

/* Locator resolves PostgreSQL tool binary paths. */
type Locator interface {
	PgDump() (string, error)
	PgRestore() (string, error)
}

/* ExecLooker delegates to exec.LookPath. */
type ExecLooker struct{}

/* LookPath finds an executable in PATH. */
func (ExecLooker) LookPath(file string) (string, error) { return exec.LookPath(file) }

/* BinDump returns the platform-specific pg_dump binary name. */
func BinDump() string {
	return binName(runtime.GOOS, "pg_dump")
}

/* BinRestore returns the platform-specific pg_restore binary name. */
func BinRestore() string {
	return binName(runtime.GOOS, "pg_restore")
}

func binName(goos, base string) string {
	if goos == "windows" {
		return base + ".exe"
	}
	return base
}

/* SystemLocator locates pg tools on PATH. */
type SystemLocator struct{ look Looker }

/* NewSystemLocator creates a PATH-based tool locator. */
func NewSystemLocator(look Looker) *SystemLocator {
	return &SystemLocator{look: look}
}

/* PgDump returns the path to pg_dump. */
func (s *SystemLocator) PgDump() (string, error) { return s.lookup(BinDump()) }

/* PgRestore returns the path to pg_restore. */
func (s *SystemLocator) PgRestore() (string, error) { return s.lookup(BinRestore()) }

func (s *SystemLocator) lookup(name string) (string, error) {
	path, err := s.look.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found in PATH: %w", name, err)
	}
	return path, nil
}
