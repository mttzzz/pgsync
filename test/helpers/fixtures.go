package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// FixtureMetadata describes expected contents of a generated fixture.
type FixtureMetadata struct {
	SchemaVersion      int            `json:"schema_version"`
	Size               string         `json:"size"`
	Seed               int64          `json:"seed"`
	ExpectedTableCount int            `json:"expected_table_count"`
	ExpectedRows       map[string]int `json:"expected_rows"`
	ExpectedSequences  []string       `json:"expected_sequences"`
}

// LoadFixtureMetadata reads a fixture metadata JSON file.
func LoadFixtureMetadata(ctx context.Context, path string) (FixtureMetadata, error) {
	if err := ctx.Err(); err != nil {
		return FixtureMetadata{}, err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- caller supplies explicit fixture metadata paths in tests.
	if err != nil {
		return FixtureMetadata{}, fmt.Errorf("read fixture metadata: %w", err)
	}
	var metadata FixtureMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return FixtureMetadata{}, fmt.Errorf("decode fixture metadata: %w", err)
	}
	return metadata, nil
}
