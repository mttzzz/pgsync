package benchmarks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteResult atomically writes result as stable pretty JSON into dir.
func WriteResult(dir string, result Result) error {
	if err := result.Validate(); err != nil {
		return err
	}
	if dir == "" {
		dir = filepath.Join("benchmarks", "results", "local")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create benchmark result dir: %w", err)
	}
	path := filepath.Join(dir, result.Fixture+".json")
	tmp, err := os.CreateTemp(dir, result.Fixture+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp benchmark result: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("encode benchmark result: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close benchmark result: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("write benchmark result: %w", err)
	}
	cleanup = false
	return nil
}
