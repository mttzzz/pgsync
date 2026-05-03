// Package main generates deterministic PostgreSQL SQL fixtures.
package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

type Options struct {
	Size string
	Seed int64
	Out  string
}

type Metadata struct {
	SchemaVersion      int            `json:"schema_version"`
	Size               string         `json:"size"`
	Seed               int64          `json:"seed"`
	ExpectedTableCount int            `json:"expected_table_count"`
	ExpectedRows       map[string]int `json:"expected_rows"`
	ExpectedSequences  []string       `json:"expected_sequences"`
}

func Generate(opts Options) (Metadata, error) {
	profile, err := profileForSize(opts.Size)
	if err != nil {
		return Metadata{}, err
	}
	if opts.Out == "" {
		return Metadata{}, fmt.Errorf("out path is required")
	}
	if err := os.MkdirAll(filepath.Dir(opts.Out), 0o750); err != nil {
		return Metadata{}, fmt.Errorf("create fixture directory: %w", err)
	}
	file, err := os.Create(opts.Out)
	if err != nil {
		return Metadata{}, fmt.Errorf("create fixture: %w", err)
	}
	defer func() { _ = file.Close() }()
	gz, err := gzip.NewWriterLevel(file, gzip.BestCompression)
	if err != nil {
		return Metadata{}, fmt.Errorf("create gzip writer: %w", err)
	}
	gz.Name = ""
	gz.ModTime = time.Unix(0, 0).UTC()
	metadata := profile.metadata(opts.Seed)
	writeSQL(gz, profile, opts.Seed)
	if err := gz.Close(); err != nil {
		return Metadata{}, fmt.Errorf("close gzip fixture: %w", err)
	}
	if err := writeMetadata(opts.Out+".json", metadata); err != nil {
		return Metadata{}, err
	}
	return metadata, nil
}

func writeMetadata(path string, metadata Metadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encode fixture metadata: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write fixture metadata: %w", err)
	}
	return nil
}

func writeSQL(w io.Writer, profile fixtureProfile, seed int64) {
	rng := rand.New(rand.NewSource(seed)) // #nosec G404 -- deterministic fixture data generation requires a seeded PRNG.
	_, _ = fmt.Fprintln(w, "-- pgsync deterministic PostgreSQL 18 fixture")
	_, _ = fmt.Fprintln(w, "SET client_min_messages = warning;")
	_, _ = fmt.Fprintln(w, "CREATE EXTENSION IF NOT EXISTS pgcrypto;")
	_, _ = fmt.Fprintln(w, "CREATE TYPE fixture_status AS ENUM ('new', 'active', 'archived');")
	_, _ = fmt.Fprintln(w, "CREATE TABLE accounts (id bigserial PRIMARY KEY, name text NOT NULL, status fixture_status NOT NULL, attrs jsonb NOT NULL DEFAULT '{}'::jsonb);")
	_, _ = fmt.Fprintln(w, "CREATE TABLE projects (id bigserial PRIMARY KEY, account_id bigint NOT NULL REFERENCES accounts(id), name text NOT NULL, tags text[] NOT NULL DEFAULT '{}');")
	_, _ = fmt.Fprintln(w, "CREATE TABLE events (id bigserial PRIMARY KEY, project_id bigint NOT NULL REFERENCES projects(id), payload jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now());")
	_, _ = fmt.Fprintln(w, "CREATE INDEX projects_account_id_idx ON projects(account_id);")
	_, _ = fmt.Fprintln(w, "CREATE INDEX events_project_id_idx ON events(project_id);")
	_, _ = fmt.Fprintln(w, "CREATE INDEX events_payload_gin_idx ON events USING gin(payload);")
	statuses := []string{"new", "active", "archived"}
	for i := 1; i <= profile.Accounts; i++ {
		_, _ = fmt.Fprintf(w, "INSERT INTO accounts(id, name, status, attrs) VALUES (%d, 'account-%06d', '%s', '{\"tier\":%d}'::jsonb);\n", i, i, statuses[rng.Intn(len(statuses))], 1+rng.Intn(5))
	}
	for i := 1; i <= profile.Projects; i++ {
		accountID := 1 + rng.Intn(profile.Accounts)
		_, _ = fmt.Fprintf(w, "INSERT INTO projects(id, account_id, name, tags) VALUES (%d, %d, 'project-%06d', ARRAY['tag-%d','tag-%d']);\n", i, accountID, i, rng.Intn(25), rng.Intn(25))
	}
	for i := 1; i <= profile.Events; i++ {
		projectID := 1 + rng.Intn(profile.Projects)
		_, _ = fmt.Fprintf(w, "INSERT INTO events(id, project_id, payload, created_at) VALUES (%d, %d, '{\"kind\":\"event-%d\",\"value\":%d}'::jsonb, now() - (%d || ' seconds')::interval);\n", i, projectID, rng.Intn(20), rng.Intn(100000), rng.Intn(86400))
	}
	_, _ = fmt.Fprintln(w, "SELECT setval('accounts_id_seq', (SELECT max(id) FROM accounts));")
	_, _ = fmt.Fprintln(w, "SELECT setval('projects_id_seq', (SELECT max(id) FROM projects));")
	_, _ = fmt.Fprintln(w, "SELECT setval('events_id_seq', (SELECT max(id) FROM events));")
}
