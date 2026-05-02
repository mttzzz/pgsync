package pgtools

import (
	"fmt"
	"slices"

	"github.com/BurntSushi/toml"
)

// Manifest describes embedded PostgreSQL tool archives.
type Manifest struct {
	SchemaVersion int                `toml:"schema_version"`
	ToolVersion   string             `toml:"tool_version"`
	Platforms     map[string]Package `toml:"platforms"`
}

// Package describes one platform archive.
type Package struct {
	URL              string   `toml:"url"`
	ArchiveSHA256    string   `toml:"archive_sha256"`
	Files            []string `toml:"files"`
	ExpectedBinaries []string `toml:"expected_binaries"`
}

// ParseManifest parses and validates a pgtools manifest.
func ParseManifest(data []byte) (Manifest, error) {
	var manifest Manifest
	if _, err := toml.Decode(string(data), &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.SchemaVersion != 1 {
		return Manifest{}, fmt.Errorf("unsupported manifest schema: %d", manifest.SchemaVersion)
	}
	if manifest.ToolVersion == "" {
		return Manifest{}, fmt.Errorf("tool_version is required")
	}
	if len(manifest.Platforms) == 0 {
		return Manifest{}, fmt.Errorf("platforms are required")
	}
	for name, pkg := range manifest.Platforms {
		if err := validatePackage(name, pkg); err != nil {
			return Manifest{}, err
		}
	}
	return manifest, nil
}

func validatePackage(name string, pkg Package) error {
	if name == "" || pkg.URL == "" || pkg.ArchiveSHA256 == "" {
		return fmt.Errorf("platform %q is incomplete", name)
	}
	if len(pkg.ExpectedBinaries) == 0 {
		return fmt.Errorf("platform %q has no expected binaries", name)
	}
	for _, bin := range pkg.ExpectedBinaries {
		if !slices.Contains(pkg.Files, bin) {
			return fmt.Errorf("platform %q missing binary %q in files", name, bin)
		}
	}
	return nil
}
