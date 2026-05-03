package pgtools

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"runtime"
	"slices"
	"strings"
)

const embeddedToolVersion = "18.3"

type embeddedBundle struct {
	Platform  string
	Version   string
	FS        fs.FS
	Root      string
	Available bool
}

// EmbeddedBundle returns the build-tagged pgtools bundle for the current platform.
func EmbeddedBundle() embeddedBundle {
	return currentEmbeddedBundle()
}

// EmbeddedAvailable reports whether the current binary has a complete embedded pgtools payload.
func EmbeddedAvailable() bool {
	return EmbeddedBundle().Available
}

// EmbeddedPlatform returns the current runtime platform in release artifact form.
func EmbeddedPlatform() string {
	return platformName(runtime.GOOS, runtime.GOARCH)
}

func newEmbeddedBundle(goos, goarch string, source fs.FS, root string) embeddedBundle {
	bundle := embeddedBundle{
		Platform: platformName(goos, goarch),
		Version:  embeddedToolVersion,
		FS:       source,
		Root:     root,
	}
	bundle.Available = embeddedBundleHasRequiredFiles(bundle)
	return bundle
}

func platformName(goos, goarch string) string {
	return goos + "-" + goarch
}

func embeddedUnavailableError(bundle embeddedBundle) error {
	if bundle.FS != nil && strings.TrimSpace(bundle.Root) != "" {
		return fmt.Errorf(
			"embedded PostgreSQL tools payload is not staged for %s; run pgtools-prepare-release before building release binaries or rerun with --use-system-pgtools",
			bundle.Platform,
		)
	}
	return fmt.Errorf(
		"embedded PostgreSQL tools are not supported for %s; install PostgreSQL client tools and rerun with --use-system-pgtools",
		bundle.Platform,
	)
}

func embeddedBundleHasRequiredFiles(bundle embeddedBundle) bool {
	files, err := embeddedBundleFiles(bundle)
	if err != nil {
		return false
	}
	for _, name := range requiredBinaryNames(bundle.Platform) {
		if _, ok := embeddedBinaryRelativePath(files, name); !ok {
			return false
		}
	}
	return true
}

func embeddedBundleFiles(bundle embeddedBundle) (map[string][]byte, error) {
	if bundle.FS == nil || strings.TrimSpace(bundle.Root) == "" {
		return nil, fmt.Errorf("embedded pgtools bundle is not available")
	}
	files := map[string][]byte{}
	if err := fs.WalkDir(bundle.FS, bundle.Root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		base := path.Base(name)
		if base == ".gitkeep" || base == "gitkeep.txt" {
			return nil
		}
		data, err := fs.ReadFile(bundle.FS, name)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(name, strings.TrimRight(bundle.Root, "/")+"/")
		files[rel] = data
		return nil
	}); err != nil {
		return nil, fmt.Errorf("read embedded pgtools bundle: %w", err)
	}
	return files, nil
}

func embeddedBundleHash(bundle embeddedBundle) (string, map[string]string, error) {
	files, err := embeddedBundleFiles(bundle)
	if err != nil {
		return "", nil, err
	}
	h := sha256.New()
	_, _ = h.Write([]byte(bundle.Platform))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(bundle.Version))
	_, _ = h.Write([]byte{0})
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	fileHashes := make(map[string]string, len(names))
	for _, name := range names {
		fileHash := sha256.Sum256(files[name])
		fileHashes[name] = hex.EncodeToString(fileHash[:])
		_, _ = h.Write([]byte(name))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(files[name])
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), fileHashes, nil
}

func requiredBinaryNames(platform string) []string {
	if strings.HasPrefix(platform, "windows-") {
		return []string{"pg_dump.exe", "pg_restore.exe"}
	}
	return []string{"pg_dump", "pg_restore"}
}

func binaryNameForPlatform(platform, base string) string {
	if strings.HasPrefix(platform, "windows-") {
		return base + ".exe"
	}
	return base
}

func binaryRelativePath(files map[string]string, binaryName string) (string, bool) {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if path.Base(name) == binaryName {
			return name, true
		}
	}
	return "", false
}

func embeddedBinaryRelativePath(files map[string][]byte, binaryName string) (string, bool) {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if path.Base(name) == binaryName {
			return name, true
		}
	}
	return "", false
}
