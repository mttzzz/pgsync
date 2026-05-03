// Package pgtools resolves paths to pg_dump and pg_restore.
package pgtools

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"sync"
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
	if s == nil || s.look == nil {
		return "", fmt.Errorf("system pgtools locator is not configured")
	}
	path, err := s.look.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found in PATH: %w", name, err)
	}
	return path, nil
}

// LocatorOptions configures selection between embedded and system PostgreSQL tools.
type LocatorOptions struct {
	UseSystem bool
	CacheRoot string
	System    Locator
	Extractor *Extractor
	Logger    *slog.Logger
}

// SelectingLocator uses embedded tools by default and system PATH tools on request.
type SelectingLocator struct {
	mu        sync.Mutex
	useSystem bool
	system    Locator
	extractor *Extractor
	logger    *slog.Logger
	paths     *Paths
}

// NewLocator creates a production pgtools locator.
func NewLocator(opts LocatorOptions) *SelectingLocator {
	system := opts.System
	if system == nil {
		system = NewSystemLocator(ExecLooker{})
	}
	extractor := opts.Extractor
	if extractor == nil {
		extractor = &Extractor{CacheRoot: opts.CacheRoot, Logger: opts.Logger}
	} else if extractor.CacheRoot == "" {
		extractor.CacheRoot = opts.CacheRoot
	}
	return &SelectingLocator{
		useSystem: opts.UseSystem,
		system:    system,
		extractor: extractor,
		logger:    opts.Logger,
	}
}

// SetUseSystem switches the locator mode for a resolved sync plan.
func (l *SelectingLocator) SetUseSystem(useSystem bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.useSystem != useSystem {
		l.paths = nil
	}
	l.useSystem = useSystem
}

// Mode reports the current locator mode.
func (l *SelectingLocator) Mode() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.useSystem {
		return "system"
	}
	return "embedded"
}

// PgDump returns the selected pg_dump path.
func (l *SelectingLocator) PgDump() (string, error) {
	paths, err := l.ensure(context.Background())
	if err != nil {
		return "", err
	}
	return paths.PgDump, nil
}

// PgRestore returns the selected pg_restore path.
func (l *SelectingLocator) PgRestore() (string, error) {
	paths, err := l.ensure(context.Background())
	if err != nil {
		return "", err
	}
	return paths.PgRestore, nil
}

// Paths returns both selected tool paths and embedded cache metadata.
func (l *SelectingLocator) Paths(ctx context.Context) (Paths, error) {
	return l.ensure(ctx)
}

func (l *SelectingLocator) ensure(ctx context.Context) (Paths, error) {
	if l == nil {
		return Paths{}, fmt.Errorf("pgtools locator is not configured")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.useSystem {
		dump, err := l.system.PgDump()
		if err != nil {
			return Paths{}, fmt.Errorf("locate system pg_dump: %w", err)
		}
		restore, err := l.system.PgRestore()
		if err != nil {
			return Paths{}, fmt.Errorf("locate system pg_restore: %w", err)
		}
		return Paths{PgDump: dump, PgRestore: restore, Platform: EmbeddedPlatform()}, nil
	}
	if l.paths != nil {
		return *l.paths, nil
	}
	bundle := EmbeddedBundle()
	if !bundle.Available {
		return Paths{}, embeddedUnavailableError(bundle)
	}
	paths, err := l.extractor.Ensure(ctx, bundle)
	if err != nil {
		return Paths{}, fmt.Errorf("extract embedded pgtools: %w", err)
	}
	l.paths = &paths
	if l.logger != nil {
		l.logger.Debug("embedded pgtools ready", "platform", paths.Platform, "root", paths.Root, "hash", paths.Hash)
	}
	return paths, nil
}
