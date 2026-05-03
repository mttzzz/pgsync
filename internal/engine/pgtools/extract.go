package pgtools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/mttzzz/pgsync/internal/runner"
)

const installMarkerName = ".installed"

// Paths contains resolved PostgreSQL tool executable paths and cache metadata.
type Paths struct {
	PgDump    string
	PgRestore string
	Root      string
	Hash      string
	Platform  string
}

// Extractor materializes embedded pgtools into a verified runtime cache.
type Extractor struct {
	Runner    runner.CommandRunner
	Logger    *slog.Logger
	CacheRoot string
	Signer    Signer
	Now       func() time.Time
}

type installMarker struct {
	SchemaVersion int               `json:"schema_version"`
	Platform      string            `json:"platform"`
	Version       string            `json:"version"`
	Hash          string            `json:"hash"`
	Files         map[string]string `json:"files"`
	CreatedAt     time.Time         `json:"created_at"`
}

// Ensure extracts bundle into the cache if needed and returns executable paths.
//
//nolint:gocognit,gocyclo // Extraction performs a linear sequence of validation, cache recovery, and atomic install steps.
func (e *Extractor) Ensure(ctx context.Context, bundle embeddedBundle) (Paths, error) {
	if err := ctx.Err(); err != nil {
		return Paths{}, err
	}
	if !bundle.Available {
		return Paths{}, fmt.Errorf("embedded PostgreSQL tools are not staged for %s; run pgtools release preparation before building or rerun with --use-system-pgtools", bundle.Platform)
	}
	files, err := embeddedBundleFiles(bundle)
	if err != nil {
		return Paths{}, err
	}
	hash, fileHashes, err := embeddedBundleHash(bundle)
	if err != nil {
		return Paths{}, err
	}
	dumpRel, ok := embeddedBinaryRelativePath(files, binaryNameForPlatform(bundle.Platform, "pg_dump"))
	if !ok {
		return Paths{}, fmt.Errorf("embedded pg_dump not found in payload for %s", bundle.Platform)
	}
	restoreRel, ok := embeddedBinaryRelativePath(files, binaryNameForPlatform(bundle.Platform, "pg_restore"))
	if !ok {
		return Paths{}, fmt.Errorf("embedded pg_restore not found in payload for %s", bundle.Platform)
	}
	root, err := e.cacheRoot()
	if err != nil {
		return Paths{}, err
	}
	finalDir := filepath.Join(root, hash)
	paths := extractedPaths(finalDir, hash, bundle.Platform, dumpRel, restoreRel)
	marker := installMarker{
		SchemaVersion: 1,
		Platform:      bundle.Platform,
		Version:       bundle.Version,
		Hash:          hash,
		Files:         fileHashes,
		CreatedAt:     e.now().UTC(),
	}
	if err := validateInstall(finalDir, marker); err == nil {
		return paths, nil
	}
	if err := os.RemoveAll(finalDir); err != nil {
		return Paths{}, fmt.Errorf("remove corrupt pgtools cache: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return Paths{}, fmt.Errorf("mkdir pgtools cache root: %w", err)
	}
	tmpDir, err := os.MkdirTemp(root, hash+"-tmp-*")
	if err != nil {
		return Paths{}, fmt.Errorf("create pgtools temp cache: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmpDir)
		}
	}()
	if err := e.writeFiles(ctx, tmpDir, bundle, files); err != nil {
		return Paths{}, err
	}
	if err := writeInstallMarker(tmpDir, marker); err != nil {
		return Paths{}, err
	}
	if err := os.Rename(tmpDir, finalDir); err != nil {
		if validateErr := validateInstall(finalDir, marker); validateErr == nil {
			cleanup = true
			return paths, nil
		}
		return Paths{}, fmt.Errorf("activate pgtools cache: %w", err)
	}
	cleanup = false
	return paths, nil
}

//nolint:gocognit,gocyclo // File extraction intentionally validates path safety, permissions, and signing inline per file.
func (e *Extractor) writeFiles(ctx context.Context, root string, bundle embeddedBundle, files map[string][]byte) error {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	var errs []error
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		clean := filepath.Clean(filepath.FromSlash(name))
		if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return fmt.Errorf("unsafe embedded pgtools path %q", name)
		}
		path := filepath.Join(root, clean)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("mkdir embedded pgtools path: %w", err)
		}
		perm := filePerm(bundle.Platform, name)
		if err := os.WriteFile(path, files[name], perm); err != nil {
			return fmt.Errorf("write embedded pgtools file %q: %w", name, err)
		}
		if err := os.Chmod(path, perm); err != nil {
			errs = append(errs, fmt.Errorf("chmod embedded pgtools file %q: %w", name, err))
			continue
		}
		if e.signer().ShouldSign(bundle.Platform, name) {
			if err := e.signer().Sign(ctx, path); err != nil {
				errs = append(errs, fmt.Errorf("sign embedded pgtools file %q: %w", name, err))
			}
		}
	}
	return errors.Join(errs...)
}

func (e *Extractor) cacheRoot() (string, error) {
	if strings.TrimSpace(e.CacheRoot) != "" {
		return e.CacheRoot, nil
	}
	return defaultCacheRoot(runtime.GOOS, os.Getenv("LOCALAPPDATA"), os.Getenv("HOME"), userHomeDir)
}

func (e *Extractor) signer() Signer {
	if e != nil && e.Signer != nil {
		return e.Signer
	}
	return NewSigner(runtime.GOOS, e.runner(), e.logger())
}

func (e *Extractor) runner() runner.CommandRunner {
	if e == nil {
		return nil
	}
	return e.Runner
}

func (e *Extractor) logger() *slog.Logger {
	if e == nil {
		return nil
	}
	return e.Logger
}

func (e *Extractor) now() time.Time {
	if e != nil && e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func defaultCacheRoot(goos, localAppData, home string, homeDir func() (string, error)) (string, error) {
	if goos == "windows" {
		if strings.TrimSpace(localAppData) == "" {
			return "", fmt.Errorf("LOCALAPPDATA is required for embedded pgtools cache on Windows")
		}
		return filepath.Join(localAppData, "pgsync", "cache"), nil
	}
	if strings.TrimSpace(home) == "" {
		var err error
		home, err = homeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory for embedded pgtools cache: %w", err)
		}
	}
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("home directory is required for embedded pgtools cache")
	}
	return filepath.Join(home, ".pgsync", "cache"), nil
}

func userHomeDir() (string, error) {
	return os.UserHomeDir()
}

func extractedPaths(root, hash, platform, dumpRel, restoreRel string) Paths {
	return Paths{
		PgDump:    filepath.Join(root, filepath.Clean(filepath.FromSlash(dumpRel))),
		PgRestore: filepath.Join(root, filepath.Clean(filepath.FromSlash(restoreRel))),
		Root:      root,
		Hash:      hash,
		Platform:  platform,
	}
}

func filePerm(platform, name string) os.FileMode {
	if strings.HasPrefix(platform, "windows-") {
		return 0o600
	}
	base := filepath.Base(name)
	for _, bin := range requiredBinaryNames(platform) {
		if base == bin {
			return 0o755
		}
	}
	if strings.Contains(base, ".so") || strings.HasSuffix(base, ".dylib") {
		return 0o755
	}
	return 0o644
}

func writeInstallMarker(root string, marker installMarker) error {
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return fmt.Errorf("encode pgtools install marker: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(root, installMarkerName), data, 0o600); err != nil {
		return fmt.Errorf("write pgtools install marker: %w", err)
	}
	return nil
}

//nolint:gocyclo // Validation checks marker metadata, required binaries, and every file hash explicitly.
func validateInstall(root string, want installMarker) error {
	data, err := os.ReadFile(filepath.Join(root, installMarkerName)) // #nosec G304 -- root is the controlled pgtools cache root.
	if err != nil {
		return fmt.Errorf("read pgtools install marker: %w", err)
	}
	var got installMarker
	if err := json.Unmarshal(data, &got); err != nil {
		return fmt.Errorf("decode pgtools install marker: %w", err)
	}
	if got.SchemaVersion != want.SchemaVersion || got.Platform != want.Platform || got.Version != want.Version || got.Hash != want.Hash {
		return fmt.Errorf("pgtools install marker metadata mismatch")
	}
	for _, bin := range requiredBinaryNames(want.Platform) {
		rel, ok := binaryRelativePath(want.Files, bin)
		if !ok {
			return fmt.Errorf("pgtools executable %s missing from marker", bin)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.Clean(filepath.FromSlash(rel)))); err != nil {
			return fmt.Errorf("pgtools executable %s missing: %w", bin, err)
		}
	}
	for name, wantHash := range want.Files {
		data, err := os.ReadFile(filepath.Join(root, filepath.Clean(filepath.FromSlash(name)))) // #nosec G304 -- names come from embedded payload metadata.
		if err != nil {
			return fmt.Errorf("read cached pgtools file %q: %w", name, err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != wantHash {
			return fmt.Errorf("cached pgtools file %q hash mismatch", name)
		}
	}
	return nil
}
